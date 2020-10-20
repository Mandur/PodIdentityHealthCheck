using Microsoft.Extensions.Diagnostics.HealthChecks;
using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace HealthChecks
{
    public class PodIdentityHealthCheck : IHealthCheck
    {
        private readonly IHttpClientFactory _clientFactory;

        public PodIdentityHealthCheck(IHttpClientFactory clientFactory)
        {
            _clientFactory = clientFactory;
        }
        public async Task<HealthCheckResult> CheckHealthAsync(
            HealthCheckContext context,
            CancellationToken cancellationToken = default(CancellationToken))
        {
            const string hostIpEnvVarName = "HOST_IP";
            var hostIp = Environment.GetEnvironmentVariable(hostIpEnvVarName);

            if(hostIp == null)
            {
                throw new KeyNotFoundException($"environment variable {hostIpEnvVarName} could not be found in the pod");
            }

            var request = new HttpRequestMessage(HttpMethod.Get, $"http://{hostIp}:8085/healthz");

            var client = _clientFactory.CreateClient();

            var response = await client.SendAsync(request);

            if (response.IsSuccessStatusCode)
            {
                return HealthCheckResult.Healthy("The NMI liveness is reponding.");
            }

            return HealthCheckResult.Unhealthy("The NMI liveness probe did not responded as expected.");
        }
    }
}
