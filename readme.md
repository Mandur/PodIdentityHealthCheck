# Pod Identity Init Container

## The problem
The [pod identity](https://github.com/Azure/aad-pod-identity) implementation for Azure Kubernetes Service (AKS) relies on the Node Managed identity (NMI) in order to answer from authentication requests coming from the pods.
In cases like cluster scale out, application pods can become available before the NMI, leading to applcation authentication requests failing generating applications error messages. Ultimately this leads to confusing container crashes and error message reported in the operation framework. In cases like cluster scale down, the same can happen if the nmi pods get killed before the application pod.

## Proposed solution

The proposed and adopted solution is to check that the NMI liveness are still indicating that the NMI pod is alive and accepting requests. We suggest two solution here below, one that is purely based on yaml, and the other using an external image as a sidecar to run necessary check. 

The recommended approach is to use the yaml way as it is more lightweight, we are offering the other solution for docker images that don't have a wget available on the container image.

Example on how to use the two different techniques can be seen in the integration tests.

### The YAML way

We are here using the Kubernetes yaml directive to check that the NMI liveness probe is indicating the pod is alive and well. Thiw approach requires wget to be available on the Docker image.

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
    livenessProbe:
      exec:
        command:
        - sh
        - -c
        - if [[ wget -qO- $HOST_IP:8085/healthz == Active ]]; then return 0; else return 1; fi
      initialDelaySeconds: 5
      periodSeconds: 5
```

### External Image

#### Quickstart

In order to use the proposed solution, you just need to add the container "mandrx/podidentityinitcheck:0.1.2" as a sidecar container in your Kubernetes deployment. Here is an example:

``` yaml
spec:
  containers:
  - name: <Your name>
    image: <your application image>
  - name: check-pod-identity
    image: mandrx/podidentityinitcheck:0.1.2
    env:
    - name: HOST_IP
      valueFrom:
        fieldRef:
          apiVersion: v1
          fieldPath: status.hostIP
```

#### Configuration 

The image accepts the following environment variables that you can specify in the Kubernetes yaml:
|  | Default | Description | 
|-|-|-|
| HTTP_RETRY_MIN_SECONDS | "3s" | Minimum time the http retry logic is going to wait before retrying |
| HTTP_RETRY_MAX_SECONDS | "10s" | Maximum time the http retry logic is going to wait before retrying |
| HTTP_RETRY_COUNT | 5 | Number of times the Http call will be retried before killing the pod |


