module github.com/ishanjainn/superopen

go 1.26.4

require (
	github.com/ishanjainn/superopen/sdk/go v0.0.0
	github.com/spf13/cobra v1.9.1
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/metric v1.43.0
	go.opentelemetry.io/otel/sdk v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
	golang.org/x/sys v0.47.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
)

replace github.com/ishanjainn/superopen/sdk/go => ./sdk/go
