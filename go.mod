module github.com/bionicotaku/kratos-template

go 1.24.0

require (
	github.com/go-kratos/kratos/v2 v2.8.0
	github.com/google/wire v0.7.0
	go.uber.org/automaxprocs v1.5.1
	google.golang.org/genproto/googleapis/api v0.0.0-20241118233622-e639e219e697
	google.golang.org/grpc v1.67.3
	google.golang.org/protobuf v1.36.8
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/bionicotaku/lingo-utils/gclog v0.0.0
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-kratos/aegis v0.2.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/form/v4 v4.2.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	go.opentelemetry.io/otel v1.29.0 // indirect
	go.opentelemetry.io/otel/metric v1.29.0 // indirect
	go.opentelemetry.io/otel/sdk v1.29.0 // indirect
	go.opentelemetry.io/otel/trace v1.29.0
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241209162323-e6fa225c2576 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/bionicotaku/lingo-utils/gclog => ../lingo-utils/gclog
