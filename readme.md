# Pod Identity Init Container

## The problem
The [pod identity](https://github.com/Azure/aad-pod-identity) implementation for Azure Kubernetes Service (AKS) relies on the Node Managed identity (NMI) in order to answer from authentication requests coming from the pods.
In cases like cluster scale out, application pods can become available before the NMI, leading to applcation authentication requests failing generating applications error messages. Ultimately this leads to confusing container crashes and error message reported in the operation framework. In cases like cluster scale down, the same can happen if the nmi pods get killed before the application pod.

## Proposed solution

Here below we propose different solutions to cope with the issue, all of them check that the NMI liveness probes are still indicating that the NMI pod is alive and accepting requests. We suggest three different solution here below: 
* Embedding the liveness check within the yaml of your running pod
* Use a custom sidecar to check on 
* Extend your application health checks to check on the NMI health probes

The recommended approach for simple cases is to use the yaml way as it is more lightweight, the sidecar solution is only for docker images that don't have 'wget' available on the container image. Example on how to use the two different techniques can be seen in the [integration tests folder](cmd/main_integration_test.go).

For complex cases where applications are dependent on multiple health check, it is advised to include the NMI pod check as part of the code base.

### The YAML way

We are here using the Kubernetes yaml directive to check on the NMI liveness probe. We are here using a conjunction of liveness and readiness probe to ensure maximum reactivity while avoiding uneccessary application pod restarts. 

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
        - if [ `wget -qO- $HOST_IP:8085/healthz` == Active ] ; then exit 0; else exit 1; fi
      initialDelaySeconds: 0
      periodSeconds: 5
    livenessProbe:
      exec:
        command:
        - sh
        - -c
        - if [ `wget -qO- $HOST_IP:8085/healthz` == Active ] ; then exit 0; else exit 1; fi
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```
The Readiness probe is very aggressive and will flag a pod as not ready to accept traffic as soon as one request to the NMI liveness probe will fail. On the other hand, the liveness probe is configured more permissively to kill the pod after 5 failed requests to the NMI liveness probes to avoid unecessary pod reboots. 
Obviously these settings are suggestion and one should adapt them with the behaviour of the applications.  

### External Image

In order to use the proposed solution, you just need to add the container "mandrx/podidentityinitcheck:0.1.2" as a sidecar container in your Kubernetes deployment. The sidecar will monitor the NMI liveness probes and kill the pod if it becomes unavalaible.

Here is an example:

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

The advantage here is clearly simplicity. An obvious downside of this solution is the additional container deployed, even if it is only ~5 mb it is an additional component deployed. Another downside is the incapacty to do a distinction between liveness and readiness as we did in the previous section.

Another option can be to use a standard image and embed the liveness and readiness probe inside it:
``` yaml
spec:
  containers:
  - name: <your container name>
    image: <your application image name>
  - name: main
    image: busybox
    command: [ "sh", "-c", "--" ]
    args: [ "while true; do sleep 30; done;" ]
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
        - if [ `wget -qO- $HOST_IP:8085/healthz` == Active ] ; then exit 0; else exit 1; fi
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```
The above expression is a bit more verbose. However it has the advantage to be even more lightweight (~3mb) and not to rely on a custom docker image. Note that here we cannot use the readiness probe as the readiness won't be carried over to the application pod.

### Add as part of the application liveness check

In case of complex situation where the application container rely on multiple health checks, it is better to add the check to the NMI liveness probe as part of the application code. The Host IP should still be seeded as environment variable in the pod's yaml but all the other logic will be handled by code. Here below an example using asp.net core, the same idea could be reimplemented using the local health check primitives.

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
        port: 8080
      initialDelaySeconds: 0
      periodSeconds: 5
    livenessProbe:
      httpGet:
        path: /api/healthz
        port: 8080
      initialDelaySeconds: 30
      periodSeconds: 5
      failureThreshold: 5
```

Health check code:

``` c#
 public class PodIdentityHealthCheck : IHealthCheck
{
  private readonly IHttpClientFactory _clientFactory;
  public PodIdentityHealthCheck(IHttpClientFactory clientFactory)
  {
      _clientFactory = clientFactory;
  }
  public async Task<HealthCheckResult> CheckHealthAsync(HealthCheckContext context, CancellationToken cancellationToken = default(CancellationToken))
  {
    const string hostIpEnvVarName = "HOST_IP";
    var hostIp = Environment.GetEnvironmentVariable(hostIpEnvVarName);

    if(hostIp == null)
    {
      throw new KeyNotFoundException($"environment variable {hostIpEnvVarName} could not be found in thepod");
    }

    var request = new HttpRequestMessage(HttpMethod.Get, $"http://{hostIp}:8085/healthz");
    var client = _clientFactory.CreateClient();
    var response = await client.SendAsync(request);

    if (response.IsSuccessStatusCode)
    {
      if (await response.Content.ReadAsStringAsync() == "Active")
      {
          return HealthCheckResult.Healthy("The NMI liveness is reponding.");
      }
    }

    return HealthCheckResult.Unhealthy("The NMI liveness probe did not responded as expected.");
  }
}
```

startup code: 

``` C#
  public class Startup
  {
    public void ConfigureServices(IServiceCollection services)
    {
        services.AddHttpClient();
        services.AddHealthChecks().AddCheck<PodIdentityHealthCheck>("pod_identity_health_check");
    }
   
    public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
    {
      if (env.IsDevelopment())
      {
          app.UseDeveloperExceptionPage();
      }

      app.UseRouting();
      app.UseEndpoints(endpoints =>
      {
          endpoints.MapHealthChecks("/healthz");
      });
      app.UseEndpoints(endpoints =>
      {
          endpoints.MapGet("/", async context =>
          {
              await context.Response.WriteAsync("Hello World!");
          });
      });
    }
  }
```

This method requires custom code and custom yaml, but it enables the deepest tailoring options. 
One could also consider separating the liveness and readiness check in code.

