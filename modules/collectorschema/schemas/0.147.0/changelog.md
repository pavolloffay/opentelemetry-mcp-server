## v0.147.0

### 🛑 Breaking changes 🛑

- `all`: Remove unmaintained receiver/bigip component (#46040)
- `exporter/elasticsearch`: Ignore `mapping::mode` config option (#45246)
  The `mapping::mode` config option has already been deprecated and is now ignored. Instead, use
  the `X-Elastic-Mapping-Mode` client metadata key (via headers_setter extension) or the
  `elastic.mapping.mode` scope attribute to control the mapping mode per-request.
  See the README for migration instructions.
  
- `extension/sumologic`: Migrate updateCollectorMetadata from feature gate to config parameter and enable by default (#46102)
  The `extension.sumologic.updateCollectorMetadata` feature gate has been replaced with an
  `update_metadata` configuration parameter. The feature is now enabled by default, eliminating
  the need for users to specify the `--feature-gates` flag. Users can control this behavior via
  the YAML configuration:
  
  ```yaml
  extensions:
    sumologic:
      update_metadata: true  # default: true
  ```
  
- `processor/azuredetector`: Changed cloud platform value for Azure VM from `azure_vm` to `azure.vm` to align with OpenTelemetry semantic conventions v1.39.0. (#45030)
- `processor/resourcedetection`: Remove feature gate processor.resourcedetection.removeGCPFaasID (#45808)
- `processor/resourcedetection`: Changed cloud platform value for Azure EKS from `azure_eks` to `azure.eks` to align with OpenTelemetry semantic conventions v1.39.0. (#45030)
- `receiver/kafka`: Remove deprecated topic and exclude_topic fields (#46232)
- `receiver/mongodb`: Add service.instance.id resource attribute and move database from resource to db.namespace metric attribute in MongoDB receiver (#45506)
  The service.instance.id attribute is a deterministic UUID v5 derived from server address and port.
  The database attribute has been moved from a resource attribute to a metric-level attribute (db.namespace) on all per-database metrics, following OpenTelemetry semantic conventions.
  This produces a single resource per MongoDB server instead of one per database, fixing Prometheus batching compatibility.
  

### 🚩 Deprecations 🚩

- `processor/k8s_attributes`: Rename internal telemetry metrics to use dots instead of underscores (#45871)

### 💡 Enhancements 💡

- `connector/grafanacloud`: Make the default configuration of the Grafana Cloud Connector support Kubernetes deployments. (#45469)
  Add "k8S.node.uid" and "k8S.cluster.name" to the default set of resource attributes evaluated by the Grafana Cloud Connector.
- `exporter/clickhouse`: Do not crash the exporter if it cannot inspect the table schemas (#46285)
  Previously setting `create_schema: false` would skip validating the actual connection on start. This was useful
  if you had unstable targets, as you could run the collector and let the sending queue/failover connector handle
  connection errors to ClickHouse.
  
- `exporter/loadbalancing`: Support metrics routing by attributes in the loadbalancing exporter (#45675)
- `exporter/prometheusremotewrite`: Add support for exporting InstrumentationScope attributes as Prometheus labels. This behavior can be disabled via the `disable_scope_info` configuration option. (#45266)
- `exporter/signalfx`: Preserve 'k8s.service.' property names on the 'k8s.service.uid' dimension instead of converting them to 'kubernetes_service_' prefix. (#46291)
- `extension/awscloudwatchmetricstreams_encoding`: Adopt encoding extension streaming contract for JSON formatted metrics (#46214)
- `extension/awscloudwatchmetricstreams_encoding`: Adopt encoding extension streaming contract for AWS CloudWatch metrics extension (#46214)
- `extension/awslogs_encoding`: Adopt encoding extension streaming contract for AWS Logs Extension encoding (#46214)
- `extension/azure_encoding`: Add processing for Azure Traces (AppAvailabilityResults, AppDependencies and AppRequests) (#41725)
- `extension/azure_encoding`: Add support for Administrative, Alert, Autoscale, Policy, Security, ServiceHealth, and ResourceHealth log categories. (#45699)
- `extension/basicauth`: Add `username_file` and `password_file` options to client_auth config, enabling file-based credentials with live rotation via file watching. (#46227)
  When set, file-based credentials take precedence over inline values.
  Files are watched for changes using fsnotify, allowing credential rotation without restarting the collector.
  
- `internal/docker`: Add Docker API version auto-negotiation when `api_version` is not specified in config (#44653)
  When `api_version` is empty or omitted, the Docker client now automatically negotiates
  the highest mutually supported API version with the daemon using `WithAPIVersionNegotiation()`.
  Existing configurations with an explicit `api_version` continue to work unchanged.
  Note: The previous default behavior pinned the API version to 1.44 when not specified.
  Now it auto-negotiates with the daemon, which may result in a different API version being used.
  
- `pkg/ottl`: Add Base64Encode function to OTTL (#46071)
- `pkg/ottl`: Added metadata access path to all OTTL contexts for accessing client request metadata (#33288)
- `pkg/translator/pprof`: Ensure correct handling of default sample/profile in conversion. (#45976)
- `processor/hetznerdetector`: Update semantic conventions to v1.39.0 and add support for cloud.platform in Hetzner detector (#45489)
- `processor/k8s_attributes`: ReplicaSet handling now supports PartialObjectMetadata, reducing cold-start memory/time compared to typed informers. (#44407)
  In in-process cold-start benchmarks (no API server), PartialObjectMetadata reduced memory by ~57–59%, exact impact depends on cluster size and selectors.
- `processor/oracleclouddetector`: Add Oracle `realm` attribute to Oracle Cloud resource detection processor (#44408)
- `processor/redaction`: Add HMAC hash functions (`hmac-sha256` and `hmac-sha512`) for GDPR-compliant pseudonymization of sensitive data like IP addresses (#45715)
  HMAC functions provide rainbow table resistant hashing by using a secret key, making it impossible to reverse-engineer original values without the key.
  This enables true pseudonymization per GDPR Article 4(5) requirements while maintaining consistency for pattern analysis.
  Configure with `hash_function: hmac-sha256` (or `hmac-sha512`) and `hmac_key: "${env:REDACTION_SECRET_KEY}"`.
  
- `processor/resourcedetection`: Added Tencent Cloud CVM resource detector to the Resource Detection Processor (#45779)
- `processor/resourcedetection`: Add `tags_from_imds` config option to EC2 detector to control instance tag retrieval method (#46046)
  Introduces the `tags_from_imds` boolean field in the EC2 detector configuration.
  When set to `true`, instance tags are fetched via IMDS, which does not require any IAM
  permissions but requires `InstanceMetadataTags=enabled` on the instance.
  When set to `false` (default), tags are fetched via the EC2 `DescribeTags` API,
  which requires the `ec2:DescribeTags` IAM permission. This is the existing behavior and
  the default to avoid breaking changes.
  
- `receiver/apache`: Enables dynamic metric reaggregation in the Apache receiver. This does not break existing configuration files. (#46348)
- `receiver/azure_event_hub`: Promote the Azure Event Hub receiver to beta stability. (#41661)
- `receiver/azureblob`: Enable the Azure Blob receiver to poll for new blobs when the Event Hub endpoint is not configured. (#45717)
  The Azure Blob receiver now supports a polling-based ingestion mode that is automatically
  enabled when no Event Hub endpoint is configured. Instead of relying on
  event-driven notifications, it periodically checks the storage container
  for new blobs to ingest.
  
- `receiver/ciscoos`: Add multi-device configuration with global scraper settings and improved config validation (#42647)
- `receiver/haproxy`: Enable dynamic metric reaggregation in the HAProxy receiver. (#46357)
- `receiver/hostmetrics`: Reduce excessive float64 precision in memory, disk, and filesystem scrapers (#46141)
- `receiver/k8s_cluster`: Define entities and relationships for Kubernetes resources in metadata.yaml. (#41080)
- `receiver/oracledb`: Enable dynamic metric reaggregation in the OracleDB receiver. (#46371)
- `receiver/postgresql`: Added support for configuring custom collection intervals for Top Query records (#45614)
- `receiver/prometheusremotewrite`: Add support for extracting exemplars from Prometheus counters (#46145)
- `receiver/splunkenterprise`: Enables dynamic metric reaggregation in the Splunk Enterprise receiver. This does not break existing configuration files. (#45396)
- `receiver/tlscheck`: Added support for JKS and PKCS12 keystore formats. (#41518)
  This enhancement allows users to utilize JKS and PKCS12 keystore formats
  for TLS configuration in the tlscheck receiver, providing greater flexibility
  in certificate management.
  

### 🧰 Bug fixes 🧰

- `exporter/loadbalancing`: Change default timeout for k8s resolver from 1s to 1m to reduce excessive Kubernetes API server load. (#33004)
  The previous 1s default caused watch connections to reconnect every second, generating big amount of requests.
  The new 1m default provides a big reduction in API load while maintaining reasonable
  endpoint discovery latency. The timeout remains configurable for users who need different behavior.
  
- `exporter/prometheusremotewrite`: [exporter/prometheusremotewrite] Fix WAL wake-up race (#45288)
- `exporter/sematext`: Add validation to require `region` and at least one `app_token` in configuration. (#39610)
- `extension/oauth2client`: Make token refresh context-aware (#45917)
  The oauth2clientauth now implements its own HTTP and gRPC middleware, so the request
  context can be passed in and honoured when refreshing tokens. This allows for cancellation
  and timeouts to be respected, as well as propagation of trace context.
  
- `internal/metadataproviders`: Fix Azure IMDS header value (#46281)
  Azure's IMDS endpoint is expecting the `Metadata` header to be `true`, not `True`, the latter causing 400s.
- `pkg/fileconsumer`: Do not evaluate the include/exclude file patterns during component start (#45988)
  Previously, a synchronous glob walk of all matching files
  was performed during component start. This blocked the collector from reporting readiness, causing pods to
  remain in Not Ready state for extended periods when many files matched the configured
  glob patterns. The call was purely for logging a warning and the result was discarded.
  The first poll cycle already performs the same glob walk, so the duplicate call has been
  removed.
  The warning "no files match the configured criteria" is no longer logged. It is still logged at debug level on the first (and each subsequent) poll.
  
- `pkg/translator/pprof`: fix attribute bug (#46336)
- `pkg/translator/prometheusremotewrite`: Fixes an issue where zero_threshold was reset to the default value for exponential histograms. (#46108)
- `processor/cumulativetodelta`: Fix memory blowup in exponential histogram delta conversion (#45927)
- `processor/resourcedetection`: IRSA and Pod Identity tokens are checked to determine if running within an EKS cluster (#45866)
- `processor/tail_sampling`: Properly remove trace id from its original batch when using `decision_wait_after_root_received` (#46004)
  The bug only causes the traces dropped too early metric to be incorrectly incremented. There is no functional change to what is sampled.
- `receiver/awscloudwatch`: Use the oldest log timestamp as the next poll start time to prevent logs from being ignored (#41122)
- `receiver/docker_stats`: Avoid cancelling docker stats request before it's done reading response (#34194)
  Resolves "context canceled" error while fetching docker stats
- `receiver/oracledb`: Fix stored procedure name showing as "." and top queries appearing with zero execution count (#46438)
  Fixed procedure name concatenation in top query SQL to return NULL instead of "." when
  no stored procedure is associated with a query. Also changed top query filtering to only
  emit queries where the execution count has increased, preventing entries with zero count.
  
- `receiver/splunkenterprise`: fixes a bug with how ad-hoc search based metrics were being recorded (#46126)

<!-- previous-version -->


## v1.53.0/v0.147.0

### 💡 Enhancements 💡

- `exporter/debug`: Output bucket counts for exponential histogram data points in normal verbosity. (#10463)
- `pkg/exporterhelper`: Add `metadata_keys` configuration to `sending_queue.batch.partition` to partition batches by client metadata (#14139)
  The `metadata_keys` configuration option is now available in the `sending_queue.batch.partition` section for all exporters.
  When specified, batches are partitioned based on the values of the listed metadata keys, allowing separate batching per metadata partition. This feature
  is automatically configured when using `exporterhelper.WithQueue()`.
  

### 🧰 Bug fixes 🧰

- `cmd/builder`: Fix duplicate error output when CLI command execution fails in the builder tool. (#14436)
- `cmd/mdatagen`: Fix duplicate error output when CLI command execution fails in the mdatagen tool. (#14436)
- `cmd/mdatagen`: Fix semconv URL validation for metrics with underscores in their names (#14583)
  Metrics like `system.disk.io_time` now correctly validate against semantic convention URLs containing underscores in the anchor tag.
- `extension/memory_limiter`: Use ChainUnaryInterceptor instead of UnaryInterceptor to allow multiple interceptors. (#14634)
  If multiple extensions that use the UnaryInterceptor are set the binary panics at start time.
- `extension/memory_limiter`: Add support for streaming services. (#14634)
- `pkg/config/configmiddleware`: Add context.Context to HTTP middleware interface constructors. (#14523)
  This is a breaking API change for components that implement or use extensionmiddleware.
- `pkg/confmap`: Fix another issue where configs could fail to decode when using interpolated values in string fields. (#14034)
  For example, a resource attribute can be set via an environment variable to a string that is parseable as a number, e.g. `1234`.
  
  (A similar bug was fixed in a previous release: that one was triggered when the field was nested in a struct,
  whereas this one is triggered when the field internally has type "pointer to string" rather than "string".)
  
- `pkg/otelcol`: The featuregate subcommand now rejects extra positional arguments instead of silently ignoring them. (#14554)
- `pkg/queuebatch`: Fix data race in partition_batcher where resetTimer() was called outside mutex, causing concurrent timer.Reset() calls and unpredictable batch flush timing under load. (#14491)
- `pkg/scraperhelper`: Log scrapers now emit log-appropriate receiver telemetry (#14654)
  Log scrapers previously emitted the same receiver telemetry as metric scrapers,
  such as the otelcol_receiver_accepted_metric_points metric (instead of otelcol_receiver_accepted_log_records),
  or spans named receiver/myreceiver/MetricsReceived (instead of receiver/myreceiver/LogsReceived).
  
  This did not affect scraper-specific spans and metrics.
  
- `processor/batch`: Fixes a bug where the batch processor would not copy `SchemaUrl` metadata from resource and scope containers during partial batch splits. (#12279, #14620)

<!-- previous-version -->
