# Pod Identity best practice : Checking NMI Health

[TL/DR](#conclusion)

## Introduction

The [pod identity](https://github.com/Azure/aad-pod-identity) implementation for Azure Kubernetes Service (AKS) enables an easy way to authenticate against Azure resource without the need to manage connection string and secrets in your deployments. Additionally it enables to associate identities at pod level granularity. It relies on two components to work properly: 
* The Node Managed Identity [*NMI*] deployed on every node on the cluster (as a daemonset) component on the Kubernetes cluster that intercepts managed identity access token requests from pods normally directed to the VM IMDS endpoint. The NMI act like a proxy, verifying the request and forwarding allowed requests to the [Instance Metadata Service](https://docs.microsoft.com/en-us/azure/virtual-machines/linux/instance-metadata-service) (IMDS) on behalf on the pod.
* The Managed Identity Controller [*MIC*] watch the Kubernetes API server to dynamically mount identity on the underlying VMs, so that the *IMDS* can authenticate with this identity against Azure Active Directory. 
[More information](https://azure.github.io/aad-pod-identity/docs/).

Pod identity enables identities to be assigned and unassigned during the initial authentication request at a pod level granularity. The NMI is playing as a gatekeeper by intercepting the request to the IMDS and -if the pod has the correct identity- issue token requests on behalf of the calling pod to the IMDS. 

Here a simplified stream: As the NMI act as a gatekeeper, it will check for the pod’s label to ensure that the correct authorization is present, otherwise the NMI will return a 404 to the requesting pod. This is where lies the value of Pod Identity: to ability to assign identity at pod level and assign it at deployment time with labels.

## Problem statement

The *NMI* component is key as it is the one doing the gatekeeper to assign the pod to a given identity. It is important to understand NMI intercept the MSAL (Microsoft Authentication Library) directed to the *IMDS* endpoint. Therefore, if for some reason the NMI is not here, the running pods will issue requests directly to the IMDS and get mounted identitites. We typically saw situations where the application became available before the MIC at cluster creation and node scale out and scale down.

What would happen in such situation widely depends on:
* How many user managed identities do you have assigned in your cluster?
* Do you have a system assigned identity in your cluster?
* Are you using the *DefaultAzureIdentity* class (see NOTE) with no identity argument and letting pod identity match the default identity? 

---
> Azure SDKs provide nice way to easily authenticate against Azure ressources using Managed identity (in C#, python, node, java). It is considered as [best practice](https://devblogs.microsoft.com/azure-sdk/best-practices-for-using-azure-sdk-with-asp-net-core/) to use in your code the *DefaultAzureCredential* class to get authorization for your application as it enables seamless transition between development and production setup. This class tries different authentication mechanism in sequence and one of them is the managed identity.
---

In cases where the NMI is not available, the request will arrive directly on the IMDS **if you have only one user assigned identity mounted on your VMSS**. As there is only one identity available, the IMDS will match the request to the default identity. In this case, even without the NMI the application pod will to continue working without any error. However, the NMI Authorization checks is going to be completely bypassed.

Things become more dire If you have multiple user assigned identities assigned to your cluster machines and don't specify the exact identity object Id in the identity request (typically when using the defaultAzureCredential without providing arguments). If you have a system assigned identity to your cluster, the request will default to this identity. If you only have user assigned identities, the IMDS won't know which one to impersonate will return a 400 error (mapped to CredentialUnavailableException in C#). As the [documentation states](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/how-to-use-vm-token#get-a-token-using-http), if there are multiple user assigned identities on the cluster `object_id`, `client_id` or `mi_res_id` needs to be specified in the request.

There are two way to solve the above problem:
* We could specify the *objectId* or *principalId* of the identity to explicitely ask for a specific identity in the code. That would require knowing passing this information at application deployment time, for example passing an environment variable from the Kubernetes yaml. However, one of the value of Pod Identity is the decoupling between the application code and the identities id, using instead CRDs to save this information on a cluster. Therefore we are not keen on this solution
* We could ensure that our application Pods run **only** when the NMI is ready to intercept requests to the IMDS. In that scenario, we fully rely on Pod identity to handle access token and couple our application lifetime to the NMI. Based on what we saw above, we believe this is how Pod identity should be operated. we investigated different solutions and will suggest different ways to achieve that. 

### Solutions

We investigated a diverse set of possible solution and we will discuss pros and cons here below. 

We followed two different strategies to deal with the issue:
* **Run application pod only when the NMI is healthy** by ensuring the NMI is alive and listening. A downside of this approach is that it would not prevent issues with pod identity that are not dependant on the NMI but on other components (e.g. a missconfiguration on the identity present in the cluster, Azure managed identity being part of a deleted resource group,... ). 
* Check the full Azure identity stack by requesting an access token to the IMDS. In order to tackle the downside, we are instead doing a full identity roundtrip check to ensure every component is working appropriately. A downside compared to the previous method this call would be typically be much more costly. 

We also experimented different ways to place these health checks:
* In an **init container**, checking during pod startup that the identity is working. An obvious downside is that we do not get any runtime error detection using this method. We can use an init container to ensure our application pod only start if the NMI or the full identity stack is ready to receive the request. This solution generates less requests as the following ones, however they can only detect problems at startup. it would not prevent issues like the NMI being decommissioned before the application pod in case of cluster scale down events.
* In application's pod **health probes**. That would continuously check for the identity stack health and stop traffic to the application should it have a problem. A downside of the previous approaches is the lack of options to detect issue that would occur during the runtime of the applicaiton pods. Kubernetes offers a nice way to assess a pod current health with [probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/). Readiness probes assess if a service can receive HTTP traffic, whereas Liveness probes assess if a pod is in a bad state and should be restarted.
* In the **application code**. Using some language primitives, we can report health of a pod directly from the code. This is typically advised if the application starts to grow and depend on multiple external components.

## Assessing NMI health

To assess NMI health, we need to check on to the NMI health probe is successfull (HTTP code = 200), the NMI is healthy as per the [pod identity probes code](https://github.com/Azure/aad-pod-identity/blob/master/pkg/probes/probes.go#L9). We also need to check for the response payload as an *Active* response indicate that the NMI changed the *IPTable* routing, *Not Active* means the request will still be routed to the IMDS as described [here](https://github.com/Azure/aad-pod-identity/issues/739)).

### Check NMI health with an init Container

We could typically do that with the following yaml to prevent a pod to start before the NMI iptables route have been set.

``` yaml
spec:
  initContainers:
  - name: init-myservice
    image: busybox:1.28
    env:
    - name: HOST_IP
      valueFrom:
        fieldRef:
          apiVersion: v1
          fieldPath: status.hostIP  
    command:
    - 'sh'
    - '-c'
    - "until wget -qO- $HOST_IP:8085/healthz; do echo waiting for NMI probe startup; sleep 2; done"
  containers:
  - name: <Your name>
    image: <your application image>
```

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/nmiHealthCheckInInitContainer.yaml) and the [test code](cmd/main_integration_test.go#L316)
**This is not one of one of the recommended solutions**, as it does not test the full identity stack and does not provide ongoing identity stack health check.

### Check NMI health with a Health Probe

You can find here below an example on how such YAML could be built. 

``` yaml
spec:
  containers:
  - name: <Your name>
    image: <your application image>
    env:
    - name: HOST_IP
      valueFrom:
        fieldRef:
          apiVersion: v1
          fieldPath: status.hostIP  
    readinessProbe:
      exec:
        command:
        - sh
        - -c
        - wget -qO- $HOST_IP:8085/healthz
      initialDelaySeconds: 0
      periodSeconds: 5
    livenessProbe:
      exec:
        command:
        - sh
        - -c
        - wget -qO- $HOST_IP:8085/healthz
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```

Note that we are here using a conjunction of liveness and readiness probe. Depending on your application it could be usefull to have an aggressive readiness probe to avoid directing http traffic to non-healthy pods but a more conservative liveness probe to avoid unessecary pod restarts. Such setup would ensure maximum reactivity while avoiding unessacery reaction to transient unavailbilities. You can see such example of probes configuration in the example above. These settings are suggestion, and one should adapt them with the behaviour of the applications.  

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/nmiHealthCheckInProbes.yaml) and the [test code](cmd/main_integration_test.go#L244)

### Check NMI Health from code

This is the recommended option when dealing with a large solution depending on multiple component. One could simply add the NMI health check as part of the code.

``` yaml
spec:
  containers:
  - name: <your container name>
    image: <your application image name>
    readinessProbe:
      httpGet:
        path: /api/healthz
        port: 80
      initialDelaySeconds: 0
      periodSeconds: 5
    livenessProbe:
      httpGet:
        path: /api/healthz
        port: 80
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```

You can find a full asp.net core example under [this folder](/NMIHealthChecks) that configures the readiness and liveness probe. The same idea could be reimplemented using the health checks primitives of other languages.
NB. Separating the liveness and readiness check in code would be better in case of a real deployment.

## Assessing health of the full identity stack

As part of the investigation, we took some time to assess if we could have a proper of not only checking health of the NMI component, but of the full identity stack. We are listing this section here as reference and **we advice people to generally prefer the NMI health checks**. The checks made here are much more costly and all present some substantial downside that we judge too high to recommend.

Also, the following methods will only work if you have no more than one identity attached to your cluster VMs or decides to indicate to which identity to connect in your identity requests. please refer to the [introduction](#introduction) for mor details.

Note that the automated tests for this section are widely unreliable as the access token can be cached by the IMDS.

### Assess health of the full identity stack by an init container and az cli
Using an init container to check the full identity stack health is the strategy officially [recommended by the Pod Identity team](https://azure.github.io/aad-pod-identity/docs/best-practices/#retry-on-token-retrieval, as per their documentation, we could change the above example to:

``` yaml
spec:
  initContainers:
  - name: init-myservice
    image: mcr.microsoft.com/azure-cli
    command:
    - sh
    - -c
    - az login --identity --debug
  containers:
  - name: <Your name>
    image: <your application image>
```

This approach has the advantage to test even further than the access token allocation. But fully do the roundtrip with Azure. However, the docker image `mcr.microsoft.com/azure-cli` is 712 mb which is an heavy price to pay. One should use this method with caution.

### Assess health of the full identity stack by an init container and get access token

**Listed as reference and not recommended**

We ask an access token directly from the IMDS before starting the pod

``` yaml
spec:
  initContainers:
  - name: init-myservice
    image: curlimages/curl:1.28
    command:
    - 'sh'
    - '-c'
    - "until curl 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/' -H Metadata:true -s; do echo waiting for NMI probe startup; sleep 2; done"
  containers:
  - name: <Your name>
    image: <your application image>
```

We would suggest to change the command to also decode the access token and check that the identity object id matches with the expected identity. Otherwise you might face some IMDS caching issue, for example as [this one if identity gets recreated](https://github.com/Azure/aad-pod-identity/issues/681).

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/identityCheckInInitContainer.yaml) and the [test code](cmd/main_integration_test.go#L292)

### Assess health of the full identity stack by an application health check

**Listed as reference and not recommended**

Similarly we could dynamically assess health of the identity stack by changing our previous init container in a probe check as below:

``` yaml
spec:
  containers:
  - name: <your container name>
    image: <your application image name>
    readinessProbe:
      exec:
        command:
        - sh
        - -c
        - curl 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https%3A%2F%2Fmanagement.azure.com%2F' -H Metadata:true -s
      initialDelaySeconds: 0
      periodSeconds: 5
    livenessProbe:
      exec:
        command:
        - sh
        - -c
        - curl 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https%3A%2F%2Fmanagement.azure.com%2F' -H Metadata:true -s
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```

You can find the [Yaml](cmd/fixtures/identityHealthCheckInHealthProbes.yaml) and the [tests](cmd/main_integration_test.go#L340).

As Above, we would recommend to change the command to also decode the access token and check that the identity object id matches with the expected identity. Otherwise you might face some IMDS caching issue, for example as [this one if identity gets recreated](https://github.com/Azure/aad-pod-identity/issues/681).

### Assess health of the full identity in the application code

**Listed as reference and not recommended**

In case of complex situation where the application container rely on multiple external dependencies, it is advised to add the NMI/Identity Stack health check as part of the application code along to other health checks. 

Let us start by assessing the state of the NMI health. Note that to access the NMI ip from the code, we need the the Host IP from Kubernetes to pod's environment variable in the pod's yaml. Here is the yaml doing that:

``` yaml
spec:
  containers:
  - name: <your container name>
    image: <your application image name>
    env:
    - name: HOST_IP
      valueFrom:
        fieldRef:
          apiVersion: v1
          fieldPath: status.hostIP
    readinessProbe:
      httpGet:
        path: /api/healthz
        port: 80
      initialDelaySeconds: 0
      periodSeconds: 5
    livenessProbe:
      httpGet:
        path: /api/healthz
        port: 80
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```

You can find [here](/IdentityHealthChecks) an asp.net core application example that configures the readiness and liveness probe. The same idea could be reimplemented using the health checks primitives of other languages.
NB. Separating the liveness and readiness check in code would be better in case of a real deployment.

As Above, we would recommend to change the command to also decode the access token and check that the identity object id matches with the expected identity. Otherwise you might face some IMDS caching issue, for example as [this one if identity gets recreated](https://github.com/Azure/aad-pod-identity/issues/681).

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/nmiHealthCheckInCode.yaml) and the [test code](cmd/main_integration_test.go#268)
**This is not one of one of the recommended solutions**, as it does not provide identity stack health check.

## Validation
The different techniques have been validated using automated test. You can check the code as example and check how they perform [integration tests folder](cmd/main_integration_test.go).
Due to the sheer complexity of the tests, the full set of test takes approximately 30 minute to complete, we would adivice to run one by one. 

## Running the tests
1. Make sure your local kubectl context points to the cluster where you want the tests to run
2. Make sure Golang is installed and ready to run
3. Edit this [file](cmd/fixtures/identity.yaml) with your identities details as describes in the [docs](https://azure.github.io/aad-pod-identity/docs/demo/standard_walkthrough/#2-create-an-identity-on-azure) 
4. Run the command `go test -timeout 30m` from within the cmd folder

The automated tests will check for the different scenarios as described earlier

## Test cases

We typically check for the following:
* [Happy case] Does the pod correctly starts if everything is set up and in place
* [Startup Check] Is the pod prevented to start if the NMI is not in place at pod startup
* [Runtime Check] Can the probes prevent traffic and terminate the pod if the NMI stop being responsive
* [Pod Identity Check] Does this method detect other problems related to pod identity (like in that case, the Azure Identity missing)

test with the following strings in their names apply to the following strategy we described earlier:
*NMIHealthCheck* is checking for the NMI probes heath as part as their liveness check
*NMIHealthCheckCustomPod* is checking for the NMI probes health as part of custom code
*IdentityCheckInInitContainer* performs an `az login --identity` as part of an init container
*NmiHealthCheckInInit* is checking NMI health as init container
*IdentityHealthCheckInSidecar* is checking the pod identity health as part of a sidecar

## Retry on token retrieval

As stated in the pod identity best practices, it can take some times until an identity is assigned on the VMSS node. NMI will keep retrying on the Token request for 80 seconds maximum. It is advised to check at the DefaultAzureCredentials implementation on any language. 

For Example in C# the default exponential retry of the ManagedIdentityCredentials class is only starting with a max retry of 3, and an initial delay of 0.8 seconds. If we look at [the default implementation](https://github.com/Azure/azure-sdk-for-net/blob/b860c1c79030dbc9519778a9636b776680f5cc95/sdk/core/Azure.Core/src/Pipeline/Internal/RetryPolicy.cs#L207) we can see that the formula take the minimum between maxDelay (default 1m) and 1<<(2*Delay*0.8) where delay is initially set to 0.8 s.
The wait will be then
1st time: 1<<(int)(1*0.8*0.8) = 1s
2nd time: 1<<(int)(2*1*0.8) = 2 s
3d time: 1<<(int)(3*2*0.8) = 16 s

Therefore the default configuration is not complying with the best practices
4 retry, 80 sec max delay.

## Conclusion

As often, there are different ways to deal with the problem with each suggested approach having advantages and disadvantages.

Please find here below a recapitulative of what we discussed above:

|  | Container is prevented to start if NMI is not ready | Container is terminated if NMI stops working | Container is terminated if other parts of pod identity are not working |
|-|-|-|-|
| NMI health check as init container | Yes | No | No |
| NMI health check as health probes | Yes | Yes | No |
| Custom health probe check on NMI health probes | Yes | Yes | No |
| Full identity check as init container | Yes | No | No |
| Full identity check as health probes | Yes | Yes | Yes |
| Full identity check as custom health check | Yes | Yes | Yes |

Our final recommandations are to rely on Pod Identity to handle to handle the access token part.

**Check on NMI Health** 
* if you have a larger solution [implement NMI health check as part of your custom code]()
* In some case, if you have a simple solution that does not rely on other external components you can also [use NMI health check as part of your own health probes](#Use-Full-identity-check-in-the-health-probes)  
