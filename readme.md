# Pod Identity Init Container

[TL/DR](#conclusion)

## The problem
The [pod identity](https://github.com/Azure/aad-pod-identity) implementation for Azure Kubernetes Service (AKS) enables an easy way to authencate against Azure resource without the need to managed connection and secrets. It relies on two components: The deployment Managed Identity Controller (MIC) that monitor the Kubernetes API Server and the daemonset Node Managed identity (NMI) that interceps authentication requests coming from the pods to add authentication information to them. [More information](https://azure.github.io/aad-pod-identity/docs/).
In some cases, we observed NMI being in non-ready state whereas the application pods where issuing identity requests. This situation leads to application crashes due to identity errors as the NMI is not anymore responding the identity requests with an access token. 
We observed this error occur mainly during cluster topology changes (e.g cluster scale out, scale down). 
For example, in case cluster scale out, application pods could become available before the NMI is in a ready state. This typically cause the application to be unable to authenticate with the necessary Azure resources (e.g. Keyvault, blobs…) and typically prevents a normal startup. Ultimately this leads to confusing container crashes and missleading error message reported in the operation framework. 
A similar problem can happen in cases like cluster scale down, when the NMI pods get decommisionned before the application pod. In this case the problem could be aggravated should the application has some grace period rules higher than the NMIs default rules (30 seconds).

## Proposed solutions

Here below we propose different solutions to cope with the issue described above. We investigated a diverse set of possible solution and we will discuss pros and cons here below. 

We followed two different strategies to deal with the issue:
* Check on the NMI probes to ensure the NMI is alive and listening. A Typical downside of this approach is that it would not prevent issues with pod identity that are not dependant on the NMI but on other components (e.g. a missconfiguration on the identity present in the cluster, azure managed identity being part of a deleted resource group,... ). I
* Check the full Azure identity stack by requesting an access token. In order to tackle the downside, we are instead doing a full identity roundtrip check to ensure every component is working appropriately. A downside compared to the previous method this call would be typically involve more https calls and roundtrips. 

Additionally, we experimented different ways to place these health checks:
* In an init container, checking during pod startup that the identity is working. An obvious downside is that we do not get any runtime error detection using this method.
* In application's pod health probes. That would continuously check for the identity stack health and stop traffic to the application should it have a problem.
* In the application code. Using some language primitives, we can report health of a pod directly from the code. This is typically advised if the application starts to grow and depend on multiple external components.

Our preferred solution are:
* [Identity check as part of your own health probes](#Use-Full-identity-check-in-the-health-probes) if you have a simple solution 
* [Identity check as part of your custom](#Application-health-check-checking-the-full-identity-framework) if you have a a larger solution depending on multiple external components 

### Use an init container check to check NMI/Identity stack health

We can use an init container to ensure our application pod only start if the NMI or the full identity stack is ready to receive the request. This solution generates less requests as the following ones, however they can only detect problems at startup. it would not prevent issues like the NMI being decommissioned before the application pod in case of cluster scale down events (as we described [earlier](#the-problem)).

Only checking the NMI health probes has the advantage of being easy to do and would typically look like this: 

``` yaml
spec:
  initContainers:
  - name: init-myservice
    image: busybox:1.28
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

Using an init container to check the full identity stack health is the strategy officially [recommended by the Pod Identity team](https://azure.github.io/aad-pod-identity/docs/best-practices/#retry-on-token-retrieval), as per their documentation, we could change the above example to:

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
We think this approach can be made better. First, `mcr.microsoft.com/azure-cli` is 712 mb whereas `busybox` is only 1.5 mb. We could change the yaml to

``` yaml
spec:
  initContainers:
  - name: init-myservice
    image: busybox:1.28
    command:
    - 'sh'
    - '-c'
    - "until curl 'http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https%3A%2F%2Fmanagemenazure.com%2F' -H Metadata:true -s; do echo waiting for NMI probe startup; sleep 2; done"
  containers:
  - name: <Your name>
    image: <your application image>
```

The curl url is the [Azure instance metadata service](https://docs.microsoft.com/en-us/azure/virtual-machines/linux/instance-metadata-service), as per the [docs](https://azure.github.io/aad-pod-identity/docs/getting-started/components/#node-managed-identity), the NMI typically intercepts those request to perform the request to ADAL. Therefore, we know that the full identity stack is healthy only if our curl request return successfully.

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/identityCheckInInitContainer.yaml) and the [test code](cmd/main_integration_test.go#L292)
**This is not one of one of the recommended solutions**, as it does not provide ongoing identity stack health check.

### Use health check to check NMI/Identity stack health

A downside of the previous approaches is the lack of options to detect issue that would occur during the runtime of the applicaiton pods. Kubernetes offers a nice way to assess a pod current health with [probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/). Readiness probes assess if a service can receive HTTP traffic, whereas Liveness probes assess if a pod is in a bad state and should be restarted.

As before, if we just want to check the NMI health state we can see that as long as request to the NMI health probe is successfull (HTTP code = 200), the NMI is healthy as per the [pod identity probes code](https://github.com/Azure/aad-pod-identity/blob/master/pkg/probes/probes.go#L9) (The response payload *Active* or *Not Active* is not important in our case. It is used for the MIC pods leader election process as described [here](https://github.com/Azure/aad-pod-identity/issues/739)).

You can find here below an example on how such Yaml could be built. 

``` yaml
spec:
  containers:
  - name: <Your name>
    image: <your application image>
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
**This is not one of one of the recommended solutions**, as it does not provide identity stack health check.

A discussed earlier, we could also check for the health of full identity stack to check for issues in the Pod Identity framework that would be unrelated to Pod identity. Note that in this case the `az login --identity` command **does not work** as it seems the Azure cli is caching the identity and therefore it would still be able to return successfully even if the nmi or pod identity is in a faulty state
You can verify those claims in our [tests](cmd/main_integration_test.go#L311) proving it and here is the [associated yaml](cmd/fixtures/identityHealthCheckInSidecarProbes)

#### Use Full identity check in the health probes
However another solution is to use curl (or another tool) to try to get an [access token](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/how-to-use-vm-token). The yaml configuration of the probes would be something like :

``` yaml
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

This solution works and proves to be the most reliant one. You can find the [Yaml](cmd/fixtures/identityHealthCheckInHealthProbes.yaml) and the [tests](cmd/main_integration_test.go#L340)  

### Use the application to check NMI/Identity stack health

In case of complex situation where the application container rely on multiple external dependencies, it is advised to add the NMI/Identity Stack health check as part of the application code. 

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

You can find [here](/HealthChecks) an asp.net core application example that configures the readiness and liveness probe. The same idea could be reimplemented using the health checks primitives of other languages.
NB. Separating the liveness and readiness check in code would be better in case of a real deployment.

This method requires custom code and custom yaml, however it enables the deepest tailoring options. This should be the preferred option in case of complex deployment, relying on multiple health checks. However, this current solution does not offer ongoing health stack checks.

This solution is tested in our automated tests, please find the associated [yaml](cmd/fixtures/nmiHealthCheckInCode.yaml) and the [test code](cmd/main_integration_test.go#268)
**This is not one of one of the recommended solutions**, as it does not provide identity stack health check.

#### Application health check checking the full identity framework
If you want to check that the full pod identity framework is working, we suggest to change the Health check to try to get an access token as described [here](https://docs.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/how-to-use-vm-token#get-a-token-using-c)

## Validation
The different techniques have been validated using automated test. You can check the code as example and check how they perform [integration tests folder](cmd/main_integration_test.go).
Due to the sheer complexity of the tests, the full set of test takes approximately 30 minute to complete, we would adivice to run one by one. 

### Running the tests
1. Make sure your local kubectl context points to the cluster where you want the tests to run
2. Make sure Golang is installed and ready to run
3. Edit this [file](cmd/fixtures/identity.yaml) with your identities details as describes in the [docs](https://azure.github.io/aad-pod-identity/docs/demo/standard_walkthrough/#2-create-an-identity-on-azure) 
4. Run the command `go test -timeout 30m` from within the cmd folder

The automated tests will check for the different scenarios as described earlier

### Test cases

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

## Conclusion

As often, there are different ways to deal with the problem with each suggested approach having advantages and disadvantages, please find here below a recapitulative of what we discussed above:

|  | Container is prevented to start if NMI is not ready | Container is terminated if NMI stops working | Container is terminated if other parts of pod identity are not working |
|-|-|-|-|
| NMI health check as init container | Yes | No | No |
| NMI health check as health probes | Yes | Yes | No |
| Custom health probe check on NMI health probes | Yes | Yes | No |
| Full identity check as init container | Yes | No | No |
| Full identity check as health probes | Yes | Yes | Yes |
| Full identity check as custom health check | Yes | Yes | Yes |

Therefore, our recommendation are:
* [Identity check as part of your own health probes](#Use-Full-identity-check-in-the-health-probes) if you have a simple solution 
* [Identity check as part of your custom](#Application-health-check-checking-the-full-identity-framework) if you have a larger solution depending on multiple external components 

