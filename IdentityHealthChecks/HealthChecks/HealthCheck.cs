using Microsoft.Extensions.Diagnostics.HealthChecks;
using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.Azure.Services.AppAuthentication;
using System.Net;
using System.IO;
using Newtonsoft.Json;

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
            try
            {
                var request = new HttpRequestMessage(HttpMethod.Get, $"http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/");
                request.Headers.Add("Metadata","true");
                var client = _clientFactory.CreateClient();
                var response = await client.SendAsync(request);
                if (response.IsSuccessStatusCode)
                {
                    var stringResponse = await response.Content.ReadAsStringAsync();
                    var responseObject = JsonConvert.DeserializeObject<Dictionary<string, object>>(stringResponse);
                    if (responseObject.TryGetValue("access_token", out var accessToken))
                    {
                        if (!string.IsNullOrEmpty(accessToken.ToString()))
                        {
                            // One should verify here that object id match the expected identity to be sure.
                            return HealthCheckResult.Healthy("The Pod Identity is able to get token as expected.");
                        }
                    }
                }
            }
            catch (Exception)
            {
            }

            return HealthCheckResult.Unhealthy("The Pod Identity is not able to get token.");
        }
    }
}
