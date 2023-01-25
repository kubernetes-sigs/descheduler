## descheduler

descheduler

### Synopsis

The descheduler evicts pods which may be bound to less desired nodes

```
descheduler [flags]
```

### Options

```
      --bind-address ip                          The IP address on which to listen for the --secure-port port. The associated interface(s) must be reachable by the rest of the cluster, and by CLI/web clients. If blank or an unspecified address (0.0.0.0 or ::), all interfaces will be used. (default 0.0.0.0)
      --cert-dir string                          The directory where the TLS certs are located. If --tls-cert-file and --tls-private-key-file are provided, this flag will be ignored. (default "apiserver.local.config/certificates")
      --client-connection-burst int32            Burst to use for interacting with kubernetes apiserver.
      --client-connection-kubeconfig string      File path to kube configuration for interacting with kubernetes apiserver.
      --client-connection-qps float32            QPS to use for interacting with kubernetes apiserver.
      --descheduling-interval duration           Time interval between two consecutive descheduler executions. Setting this value instructs the descheduler to run in a continuous loop at the interval specified.
      --disable-metrics                          Disables metrics. The metrics are by default served through https://localhost:10258/metrics. Secure address, resp. port can be changed through --bind-address, resp. --secure-port flags.
      --dry-run                                  execute descheduler in dry run mode.
  -h, --help                                     help for descheduler
      --http2-max-streams-per-connection int     The limit that the server gives to clients for the maximum number of streams in an HTTP/2 connection. Zero means to use golang's default.
      --kubeconfig string                        File with kube configuration. Deprecated, use client-connection-kubeconfig instead.
      --leader-elect                             Start a leader election client and gain leadership before executing the main loop. Enable this when running replicated components for high availability.
      --leader-elect-lease-duration duration     The duration that non-leader candidates will wait after observing a leadership renewal until attempting to acquire leadership of a led but unrenewed leader slot. This is effectively the maximum duration that a leader can be stopped before it is replaced by another candidate. This is only applicable if leader election is enabled. (default 2m17s)
      --leader-elect-renew-deadline duration     The interval between attempts by the acting master to renew a leadership slot before it stops leading. This must be less than the lease duration. This is only applicable if leader election is enabled. (default 1m47s)
      --leader-elect-resource-lock string        The type of resource object that is used for locking during leader election. Supported options are 'leases', 'endpointsleases' and 'configmapsleases'. (default "leases")
      --leader-elect-resource-name string        The name of resource object that is used for locking during leader election. (default "descheduler")
      --leader-elect-resource-namespace string   The namespace of resource object that is used for locking during leader election. (default "kube-system")
      --leader-elect-retry-period duration       The duration the clients should wait between attempting acquisition and renewal of a leadership. This is only applicable if leader election is enabled. (default 26s)
      --logging-format string                    Sets the log format. Permitted formats: "text", "json". Non-default formats don't honor these flags: --add-dir-header, --alsologtostderr, --log-backtrace-at, --log_dir, --log_file, --log_file_max_size, --logtostderr, --skip-headers, --skip-log-headers, --stderrthreshold, --log-flush-frequency.\nNon-default choices are currently alpha and subject to change without warning. (default "text")
      --otel-collector-endpoint string           Set this flag to the OpenTelemetry Collector Service Address
      --otel-sample-rate float                   Sample rate to collect the Traces (default 1)
      --otel-service-name string                 OTEL Trace name to be used with the resources (default "descheduler")
      --otel-trace-namespace string              OTEL Trace namespace to be used with the resources
      --otel-transport-ca-cert string            Path of the CA Cert that can be used to generate the client Certificate for establishing secure connection to the OTEL in gRPC mode
      --permit-address-sharing                   If true, SO_REUSEADDR will be used when binding the port. This allows binding to wildcard IPs like 0.0.0.0 and specific IPs in parallel, and it avoids waiting for the kernel to release sockets in TIME_WAIT state. [default=false]
      --permit-port-sharing                      If true, SO_REUSEPORT will be used when binding the port, which allows more than one instance to bind on the same address and port. [default=false]
      --policy-config-file string                File with descheduler policy configuration.
      --secure-port int                          The port on which to serve HTTPS with authentication and authorization. If 0, don't serve HTTPS at all. (default 10258)
      --tls-cert-file string                     File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert). If HTTPS serving is enabled, and --tls-cert-file and --tls-private-key-file are not provided, a self-signed certificate and key are generated for the public address and saved to the directory specified by --cert-dir.
      --tls-cipher-suites strings                Comma-separated list of cipher suites for the server. If omitted, the default Go cipher suites will be used. 
                                                 Preferred values: TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256, TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA, TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA, TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305, TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, TLS_RSA_WITH_AES_128_CBC_SHA, TLS_RSA_WITH_AES_128_GCM_SHA256, TLS_RSA_WITH_AES_256_CBC_SHA, TLS_RSA_WITH_AES_256_GCM_SHA384. 
                                                 Insecure values: TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256, TLS_ECDHE_ECDSA_WITH_RC4_128_SHA, TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA, TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256, TLS_ECDHE_RSA_WITH_RC4_128_SHA, TLS_RSA_WITH_3DES_EDE_CBC_SHA, TLS_RSA_WITH_AES_128_CBC_SHA256, TLS_RSA_WITH_RC4_128_SHA.
      --tls-min-version string                   Minimum TLS version supported. Possible values: VersionTLS10, VersionTLS11, VersionTLS12, VersionTLS13
      --tls-private-key-file string              File containing the default x509 private key matching --tls-cert-file.
      --tls-sni-cert-key namedCertKey            A pair of x509 certificate and private key file paths, optionally suffixed with a list of domain patterns which are fully qualified domain names, possibly with prefixed wildcard segments. The domain patterns also allow IP addresses, but IPs should only be used if the apiserver has visibility to the IP address requested by a client. If no domain patterns are provided, the names of the certificate are extracted. Non-wildcard matches trump over wildcard matches, explicit domain patterns trump over extracted names. For multiple key/certificate pairs, use the --tls-sni-cert-key multiple times. Examples: "example.crt,example.key" or "foo.crt,foo.key:*.foo.com,foo.com". (default [])
```

### SEE ALSO

* [descheduler version](descheduler_version.md)	 - Version of descheduler

