# Pod Identity Init Container

## The problem
The pod identity implementation on AKS relies on the Node Managed identity (NMI) in order to answer from authentication requests coming from the pods.
In cases like cluster scale out, the pods can be available before the NMI. This leads to the authentication requests failing and -generally- error messages and container crashes that will be reported in your operation framework.

## Proposed solution

The proposed solution is to have a lightweight init container that check that the NMI is up and running and correct identity is seeded to the pod. In order to achieve that, the init container is trying to access the http://169.254.169.254/metadata/identity/oauth2/token url of the nmi to request a token to access the standard Azure management plane. The init container will succeed if the return code is 200, otherwise the init container will fail and the application pod will not be started.
It is worth noting that if the AzureIdentity and AzureIdentityBinding are not set properly, the init container will still start.

### Quickstart

In order to use the proposed solution, you just need to add the container "mandrx/podidentityinitcheck:0.0.1" as an init container in your Kubernetes deployment. Here is an example:

``` yaml
spec:
  containers:
  - name: <Your name>
    image: <your application image>
  initContainers:
  - name: check-pod-identity
    image: mandrx/podidentityinitcheck:0.0.1
```

### Configuration 

The image accepts the following environment variables that you can specify in the Kubernetes yaml:
|  | Default | Description | 
|-|-|-|
| POD_IDENTITY_URL | http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01 | URL at which the NMI is expected to intercept the authentication request |
| POD_IDENTITY_ACCESS_URL | https://management.azure.com/ | URL the authentication request is asking authorization for |
| HTTP_RETRY_MIN_SECONDS | "3s" | Minimum time the http retry logic is going to wait before retrying |
| HTTP_RETRY_MAX_SECONDS | "10s" | Maximum time the http retry logic is going to wait before retrying |
| HTTP_RETRY_COUNT | 5 | Number of times the Http call will be retried before killing the pod |

## Future work

A similar problem might arise when scaling down the node count. In that case, if the NMI terminates before the application pod, this might result again in multiple error messages. A simple solution to avoid to issue would be to ping the NMI URL from the application code to regularly check the health of the underlying NMI instance.
We are evaluating option on how we could implement this as part of a similar solution as the one here above.

