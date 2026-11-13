# Service mTLS sidecar

`cmd/service-mtls-proxy` is an optional sidecar for the gateway/resource service
link. It adds certificate validation and an exact SPIFFE URI identity check;
it does not replace either backend's dedicated service bearer authentication.
No certificates or private keys are bundled with the repository.

Build with `go build -o service-mtls-proxy ./cmd/service-mtls-proxy`.

## Ingress and egress

Ingress accepts TLS 1.3 with a verified client certificate from the configured
CA and exactly the configured peer URI. Its backend must be an explicit HTTP
loopback URL. Egress accepts HTTP only on an explicit loopback listener and
forwards only to a fixed HTTPS upstream. It presents its client certificate,
verifies the upstream CA and DNS/IP SAN, and requires the configured server URI.
Certificate lifetime and purpose are verified in both directions, including
resumed sessions. System HTTP proxy variables are not inherited.

The gateway role allows only the existing service metric, snapshot, catalog,
and isolated channel-probe routes. The resource role allows only catalog,
protection, hard-failure, observation and attempt-metadata POST routes. Customer
relay and management routes are unavailable. Redirects are rejected, encoded or
unclean paths are rejected, and untrusted forwarding headers are replaced.
Bearer headers and existing node headers continue to reach the backend.

The certificate URI identifies the service peer. Backend source credentials,
`NODE_NAME`, `X-Protection-Node`, body identity and endpoint generation retain
their existing independent checks; a TLS URI is not a substitute for them.

## Example gateway host

Use a private application listener (`STARHUB_BIND_ADDRESS=127.0.0.1`,
`STARHUB_PORT=3000`). The same setting is available as `server.bind_address` in
startup YAML. Serve customer HTTPS separately, denying the service routes on
that public listener. Firewall rules must prevent access to raw application
ports. The unchanged default application bind address is all interfaces.

The following commands use placeholder names and operator-provided files.
Run each sidecar under the deployment's service manager, with certificate key
files readable only by its service account.

```sh
service-mtls-proxy -direction ingress -role gateway -listen 0.0.0.0:8443 -upstream http://127.0.0.1:3000 -cert /run/secrets/gateway-server.pem -key /run/secrets/gateway-server.key -ca /run/secrets/resource-client-ca.pem -peer-uri spiffe://starhub/resource
service-mtls-proxy -direction egress -role resource -listen 127.0.0.1:8082 -upstream https://resource.example:8443 -cert /run/secrets/gateway-client.pem -key /run/secrets/gateway-client.key -ca /run/secrets/resource-server-ca.pem -peer-uri spiffe://starhub/resource
```

Set `RESOURCE_CATALOG_NOTIFY_URL=http://127.0.0.1:8082`. Preserve the dedicated
service token and stable `NODE_NAME` configuration described in
[RESOURCE_PLATFORM.md](RESOURCE_PLATFORM.md).

## Example resource host

Keep the resource application port private using its deployment network/firewall
policy. These examples assume its backend is reachable on loopback port 3001.

```sh
service-mtls-proxy -direction ingress -role resource -listen 0.0.0.0:8443 -upstream http://127.0.0.1:3001 -cert /run/secrets/resource-server.pem -key /run/secrets/resource-server.key -ca /run/secrets/gateway-client-ca.pem -peer-uri spiffe://starhub/gateway/node-a
service-mtls-proxy -direction egress -role gateway -listen 127.0.0.1:8083 -upstream https://gateway.example:8443 -cert /run/secrets/resource-client.pem -key /run/secrets/resource-client.key -ca /run/secrets/gateway-server-ca.pem -peer-uri spiffe://starhub/gateway/node-a
```

Point the resource publisher at its loopback egress URL. This sidecar accepts
one configured peer URI per listener. Provision separate listeners or an
approved service mesh policy when admitting multiple gateway identities.
Certificates must include the appropriate client/server authentication purpose,
the expected URI SAN, and server DNS/IP SANs matching the upstream URL. Rotate
files and restart sidecars through the deployment's normal lifecycle; automatic
certificate issuance or live certificate reload is not implemented.

## Local verification and deployment acceptance

`go test ./internal/servicetls ./config` uses an ephemeral local CA and real TLS
handshakes. It verifies a complete egress/ingress request, independent backend
bearer rejection, missing/untrusted/expired/wrong-purpose/wrong-identity client
certificates, invalid server CA/lifetime/DNS/URI, plaintext rejection, route
isolation, redirect rejection, and private-listener configuration. Invalid
certificate attempts must never reach the backend.

These checks do not certify a deployed network or a live provider. T10/V01 still
require operator evidence from the actual deployment: the issued identities and
CA policy, refusal of invalid peers at both ingress points, inaccessible raw
ports, public/service listener separation, credential rotation, restart/retry
behavior, and actual resource/provider interoperability and load. No deployment
or external acceptance result is inferred from the local fixtures.
