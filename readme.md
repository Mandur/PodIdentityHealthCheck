# Pod Identity best practice : Checking NMI Health

[TL/DR](#conclusion)

## Introduction

The [pod identity](https://github.com/Azure/aad-pod-identity) implementation for Azure Kubernetes Service (AKS) enables an easy way to authenticate against Azure resources without the need to manage connection string and secrets in your deployments. Additionally, it enables to associate identities at pod level granularity. It relies on two components to work properly: 
* The Node Managed Identity [*NMI*] deployed on every node on the cluster (as a daemonset) component on the Kubernetes cluster that intercepts managed identity access token requests from pods normally directed to the VM IMDS endpoint. The NMI acts like a proxy, verifying the request and forwarding allowed requests to the [Instance Metadata Service](https://docs.microsoft.com/en-us/azure/virtual-machines/linux/instance-metadata-service) (IMDS) on behalf on the pod.
* The Managed Identity Controller [*MIC*] watches the Kubernetes API server to dynamically assign identities on the underlying VMs, so that the *IMDS* can authenticate with a specific identity against Azure Active Directory. 
[More information](https://azure.github.io/aad-pod-identity/docs/).

Pod identity enables identities to be assigned and unassigned during the initial authentication request at a pod level granularity. The NMI is playing as a gatekeeper by intercepting the request to the IMDS and -if the pod has the correct identity- by issueing token requests on behalf of the calling pod to the IMDS. (see picture below)

![Pod Identity intercepting auth requests to IMDS](img/podIdentityWorking.png)

As the NMI acts as a gatekeeper, it will check for the pod’s label to ensure that the correct authorization is present, otherwise the NMI will return a 404 to the requesting pod. (image below)

![Pod Identity intercepting auth requests from not authorized pods](img/podIdentityUnauthorized.png)

This is where lies the value of Pod Identity: The ability to assign identities at pod level using labels and provision them decoupled from the application deployment.

## Problem statement

The *NMI* component is key as it is the one doing the gatekeeper to assign the pod to a given identity. It is important to understand NMI intercepts the MSAL (Microsoft Authentication Library) requests directed to the *IMDS* endpoint by changing the iptables to reroute auth requests. Therefore, if for some reason the NMI is unhealthy, the running pods will issue requests directly to the IMDS. 

We have seen cases when application pods were available and the NMI was not (i.e. events like cluster creation, machine scale up or scale down). This could happen as Kubernetes does not have any native mechanisms to ensure running workloads before another one, therefore it can happen the application initiates authentication requests before the NMI is ready.

What would happen in such situation widely depends on:
* How many user managed identities do you have assigned in your cluster?
* Do you have a system assigned identity in your cluster?
* Are you using the *DefaultAzureIdentity* class (see NOTE) with no identity argument and letting pod identity match the default identity? 

---
> Azure SDKs provide nice way to easily authenticate against Azure ressources using Managed identity (in C#, python, node, java). It is considered as [best practice](https://devblogs.microsoft.com/azure-sdk/best-practices-for-using-azure-sdk-with-asp-net-core/) to use in your code the *DefaultAzureCredential* class to get authorization for your application as it enables seamless transition between development and production setup. This class tries different authentication mechanism in sequence and one of them is the managed identity.
---

If there is only one identity assigned to the underlying machines, the IMDS will match the request to this default identity. In this case, even without the NMI the application pod will continue working without any error. However, the NMI Authorization checks is going to be completely bypassed.

Things become more complicated if there are multipe identities assigned to your cluster machines and do not specify the exact identity object Id in the identity request (using the *defaultAzureCredential* without providing arguments). First, there is a system assigned identity to your cluster, the IMDS will always default to this identity whatever are the other user-assigned managed identities assigned on the nodes. (Picture below)

![IMDS defaults to the system assigned identity](img/imdsDefaultToSystemAssignedIdentity.png)

If you only have user assigned identities, the IMDS will not know which one to impersonate will return a 400 error (mapped to *CredentialUnavailableException* in C#). As the [documentation states](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/how-to-use-vm-token#get-a-token-using-http), if there are multiple user assigned identities on the cluster `object_id`, `client_id` or `mi_res_id` needs to be specified in the request. (Picture below)

![IMDS don't know which identity to resolve if multiple identities](img/imdsNoSystemAssignedIdentity.png)

In such cases, it can happen that your application receives authentication errors. Either because the access token request fails or because it gets the system-assigned identity instead of the identity defined in the pod's label.

### Solutions

There are two way to solve the above problem:
* We could specify the *objectId* or *principalId* in the authorization request to explicitely ask for a specific identity from code. However, that would require knowing passing this information at application deployment time, for example passing an environment variable from the Kubernetes yaml. However, such strategy would compromise the decoupling between the application code and the identities id enabled by Pod Identity. In addition to map the identities in the CRDs for pod identity (usually at infrastructure deployment time), we would also need to pass identities id in our application deployment. We are therefore not keen on this solution.
* We could ensure that our application Pods run **only** when the NMI is ready to intercept requests to the IMDS. In that scenario, we fully rely on Pod identity to handle access token and couple our application lifetime to the NMI. Based on what we saw above, we believe this is how Pod identity should be operated. we investigated different solutions and will suggest different ways to achieve that. We investigated a diverse set of possible solution and we will discuss pros and cons here below. 

We followed two different potential ways to deal with the issue:
* **Run application pod only when the NMI is healthy** by ensuring the NMI is alive and listening. A downside of this approach is that it would not prevent issues with pod identity that are not dependant on the NMI but on other components (e.g. a misconfiguration on the identity present in the cluster, Azure managed identity being part of a deleted resource group... ). 
* To detect problems not directly caused by the NMI, we investigated ways to check the **full Azure identity stack** by requesting an access token to the IMDS. This method is more costly than the previous, as it involves extra cluster components to work. Additionally, as we are typically issuing auth requests, it could happen than NMI is unhealthy and it goes directly to IMDS. That would result in the behavior [described previously](#problem-statement). **Therefore, we would not recommend these solutions**. However, it contains interesting findings and enables to experience the claims made above therefore, we kept it for reference. 

We also experimented different options on where to place these health checks:
* In an **init container**, checking during pod startup that the identity is working. An init container would ensure our application pod only starts if the NMI or the full identity stack is ready to receive the request. This solution generates less requests as the following ones, as they occur at startup only. An obvious downside is that we do not get any runtime health assessment detection. Therefore, it would not prevent error caused by the NMI being decommissioned before the application pod during cluster scale down events.
* In application's pod **health probes** continuously checking for the identity stack health and stop traffic to the application in case of problem. Unlike the init container, [health probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) can offer runtime health verification at the cost of having those request fired as part of the pod runtime. Readiness probes decides if an application pod can receive HTTP traffic, whereas Liveness probes assess if a pod is in a bad state and should be restarted. A combination of both is typically recommended.
* In the **application code**. Using some language primitives, we can report health of a pod as an endpoint checked by the pod's health probe as the previous method. It enables much more health check than direct health probe programming. **This is the preferred method for any productive application** that would typically depend on >1 multiple external components.

## Assessing NMI health

As described earlier, we can decide to fully rely on the NMI being there to carry our pod identity checks. One easy way to do it is to check the NMI health probe are alive. As per the [pod identity probes code](https://github.com/Azure/aad-pod-identity/blob/master/pkg/probes/probes.go#L9), it is **not enough** to check that the NMI health probe request succeed with 200 response code. We also need to check that the response payload is *Active* indicating that the NMI changed the *IPTable* routing and is ready to intercept requests made to IMDS as described [here](https://github.com/Azure/aad-pod-identity/issues/739)).

### Check NMI health with an init Container

We could perform a check as part of an init container to prevent a pod to start before the NMI iptables route have been set.

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
NB. the example above is with wget, the test yaml with curl, both approaches work, and we keep different as reference.

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/nmiHealthCheckInInitContainer.yaml) and the [test code](cmd/nmi_health_integration_test.go).

This is a good solution, but it does not offer any runtime protection therefore we would rather suggest looking at the next option.

### Check NMI health with a Health Probe

To make up with the shortcomings of the previous solution, we could perform in a liveness check in our application pod's health probes.

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

Note that we are here using a conjunction of liveness and readiness probe. Depending on your application we advice an aggressive readiness probe to avoid directing http traffic to non-healthy pods but a more conservative liveness probe to avoid unnecessary pod restarts. Such setup would ensure maximum reactivity while avoiding unnecessary reaction to transient unavailabilities. You can see such example of probes configuration in the example above. These settings are a suggestion, and one should adapt them with the behaviour of the applications. Also, some external requirements might have their own set of best practices and recommendations. 

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/nmiHealthCheckInProbes.yaml) and the [test code](cmd/nmi_health_integration_test.go)

### Check NMI Health from code

When the application grows and the application depend on multiple other components to perform normally, the method described above is not sufficient. In that case, it is advised tu use application primitives to construct a health application endpoint, aggregating multiple components' and application parts' health into a single endpoint. This endpoint would then be checked by the health probe. The YAML look similarly as above.

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

You can find a full asp.net core example under [this folder](/NMIHealthChecks). They can be done in other languages using the health checks primitives.
NB. Separating the liveness and readiness check in code would be better in case of a real deployment.

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/nmiHealthCheckInProbes.yaml) and the [test code](cmd/nmi_health_integration_test.go)

## Assessing health of the full identity stack

In the methods described above, we were tying our application lifetime to the NMI component to ensure the identity mediation was performed by the pod identity. We could still get exception external to pod identity, such as if an [identity is not yet assigned on a physical machine](https://azure.github.io/aad-pod-identity/docs/best-practices/#retry-on-token-retrieval) or if someone accidentaly delete a user assigned identity on Azure. We decided to explore if there was an easy to realiably assess the health of the full identity stack.

Spoiler: we were unable to come up with a satisfactory solution and are listing this section here as reference. Additionally these sets of method depend heavily on the cluster configuration as described in the [problem statement](#problem-statement): Do you have more than one user assigned identity on the cluster? Do you have a system assigned identity assigned on the cluster? Please refer to the [introduction](#introduction) for more details.

**we advice people to generally prefer the NMI health checks**. The checks made here are much more costly than the previous, therefore unlike previously, **we prefer init containers**

### Assess health of the full identity stack by an init container getting and Azure access token

**Listed as reference and not recommended**

We can condition the start of our application container with the fact that the application can make a successfull access token request to the IMDS. 

``` yaml
spec:
  initContainers:
  - name: init-myservice
    image: curlimages/curl:1.28
    command:
    - 'sh'
    - '-c'
    - "until curl 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/' -H Metadata:true --fail -s; do echo waiting for NMI probe startup; sleep 2; done"
  containers:
  - name: <Your name>
    image: <your application image>
```

if you want to use this strategy in production, **we strongly advice to change the command to also decode the access token and check that the identity object id matches with the expected identity**. Otherwise your request might return a different identites (e.g. the machines' system-assigned identity), or face some IMDS caching issue, for example as [this one if identity gets recreated](https://github.com/Azure/aad-pod-identity/issues/681).

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/identityCheckInInitContainer.yaml) and the [test code](cmd/nmi_health_integration_test.go)

### Assess health of the full identity stack by an init container and az cli
Using an az cli call from an init container to check the full identity stack health is the strategy officially [recommended by the Pod Identity team](https://azure.github.io/aad-pod-identity/docs/best-practices/#retry-on-token-retrieval)

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

It works very well and test a full roundtrip to azure -not just token allocation-. This is much more costly than just getting the token as there are requests made to the Azure cloud. Additionally the docker image used for the init container is `mcr.microsoft.com/azure-cli` which is 712 mb. This is a very heavy container compared with the one we use for wget (busybox ~1.5 mb) or curl (curlimages/curl ~15 mb). One should consider that higher microservice footprint might impact pod startup time and general agility on the cluster and consider this method carefully.

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/identityCheckInInitContainer.yaml) and the [test code](cmd/identity_health_integration_test.go)

### Assess health of the full identity stack by an init container

**Listed as reference and not recommended**

We could condition the pod start with the capability of wheter or not the pod is able to successfully get an access token from NMI/IMDS. As this is an init container it will not offer any runtime protection.

``` yaml
spec:
  terminationGracePeriodSeconds: 5
  initContainers:
  - name: init-myservice
    image: curlimages/curl
    env:
    - name: HOST_IP
      valueFrom:
        fieldRef:
          apiVersion: v1
          fieldPath: status.hostIP
    command:
    - 'sh'
    - '-c'
    - while ! curl --fail 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/' -m 5 -H Metadata:true -s; do echo waiting for access token; sleep 5; done
  containers:
    ...
```

if you want to use this strategy in production, **we strongly advice to change the command to also decode the access token and check that the identity object id matches with the expected identity**. Otherwise your request might return a different identites (e.g. the machines' system-assigned identity), or face some IMDS caching issue, for example as [this one if identity gets recreated](https://github.com/Azure/aad-pod-identity/issues/681).

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/identityCheckInInitContainer.yaml) and the [test code](cmd/identity_health_integration_test.go)

### Assess health of the full identity stack by an application health check

**Listed as reference and not recommended**

To come up with the shortage of the method proposed above, we could check if the application is able to successfully get an access token from NMI/IMDS. Note that the `az login` strategy does not work as health probe, it seems az has an internal token caching.

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
        - curl 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/' -H Metadata:true -s --fail'
      initialDelaySeconds: 0
      periodSeconds: 5
    livenessProbe:
      exec:
        command:
        - sh
        - -c
        - curl 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/' -H Metadata:true -s --fail'
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```

if you want to use this strategy in production, **we strongly advice to change the command to also decode the access token and check that the identity object id matches with the expected identity**. Otherwise your request might return a different identites (e.g. the machines' system-assigned identity), or face some IMDS caching issue, for example as [this one if identity gets recreated](https://github.com/Azure/aad-pod-identity/issues/681).

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/identityHealthCheckInHealthProbes.yaml) and the [test code](cmd/identity_health_integration_test.go)

### Assess health of the full identity in the application code

**Listed as reference and not recommended**

In case of complex situations where the application container relies on multiple external dependencies, it is advised to add the NMI/Identity Stack health check as part of the application code along to other health checks. 

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

if you want to use this strategy in production, **we strongly advice to change the command to also decode the access token and check that the identity object id matches with the expected identity**. Otherwise your request might return a different identites (e.g. the machines' system-assigned identity), or face some IMDS caching issue, for example as [this one if identity gets recreated](https://github.com/Azure/aad-pod-identity/issues/681).

You can find [here](/IdentityHealthChecks) an asp.net core application example that configures the readiness and liveness probe. The same idea could be reimplemented using the health checks primitives of other languages.
NB. Separating the liveness and readiness check in code would be better in case of a real deployment.

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/identityHealthCheckInCode.yaml) and the [test code](cmd/identity_health_integration_test.go)

## Validation
The different approached have been validated using automated tests that you can replicate. 
The experiments are split in two sections as the rest of the document
* Tests on [NMI strategies](cmd/nmi_health_integration_test.go). These tests are stable and hightlight our recommended solution.
* Tests on [full identity strategies](cmd/identity_health_integration_test.go). This set of tests are **not stable** and highlight why do not recommend those strategies. Tests are configured 

Due to the sheer complexity and size of the tests, the full set of test takes approximately 50 minutes to complete. We would advice to run them one by one, as there could be some race conditions with the underlying infrastructure operation. 

### Running the tests
1. Make sure your local kubectl context points to the cluster where you want the tests to run
2. Make sure Golang is installed and ready to run
3. [Install and configure pod identity](https://azure.github.io/aad-pod-identity/docs/demo/standard_walkthrough) at least once on your cluster
4. Run the tests one by one. (running the full set can take up to 50m)

### Test cases
We check for the following:
* [Happy case] Does the pod correctly starts if everything is set up and in place. (done as part of the runtime check)
* [Startup Check] Is the pod prevented to start if the NMI is not in place at pod startup (in case of health prove, ensure the pod is never in ready state)?
* [Runtime Check] Can the probes prevent traffic and terminate the pod if the NMI stop being responsive?

For the identity checks we also add the following tests. As part of these tests we deploy two containers on the cluster, one with the identity labels (to make sure the identity is assigned on the VMSS, one without labels that is used for the tests)
* [Can pod without label access identity when NMI is up?]
* [Can pod without label access identity when NMI is down?]
As described in the [introduction](#introduction), these tests are complex and rely on a cluster state. The setting `isMultiUserAssignedIdentityCluster` should be set to false if there is either a system-assigned managed identity or a single user-assigned managed identity (default). You can set it to true to change the test behavior and illustrate difference of the probes' behaviors. Typically, you will see the test [Can pod without label access identity when NMI is down?] fail as the IMDS will be able to resolve token to the default identity. In order to reduce test flakiness, it is recommended to manually add two user-assigned identites (different from the one used by pod identity). 

## Conclusion

We explored above different strategies:
* Tie the application lifetime to pod identity's NMI [Advised solution]
* Use az login as init probe as suggested in best practices [works, but has high footprint]
* Get Access token [work very unreliably due to different factors **not recommended**]

As we saw in the [Full Identity Stack](#Assessing-health-of-the-full-identity-stack), a lot of uncertainties happen when pod identity is not available. In that case, it can even happen that your application pod gets a different identity that the one assigned in pod identity, resulting in authentication exceptions. Therefore, when using pod identity, we think the best solution is to always check for the NMI health during your application pod lifetime and be very aggressive on the readiness probe to remove HTTP traffic should the NMI become unresponsive.

In case of productive solution, we recommend having the NMI health check hosted within your code as you should have other health check. In case of very simple solution, a health probe approach is good enough. The init container is not recommended as it does not prevent problems in case of cluster downscale.