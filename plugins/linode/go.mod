module github.com/guilhem/token-renewer/plugins/linode

go 1.24.2

require (
	github.com/guilhem/token-renewer v0.0.0-20250429003919-ef7d7f20bad3
	github.com/linode/linodego v1.49.0
	google.golang.org/grpc v1.72.0
)

require (
	github.com/go-resty/resty/v2 v2.16.5 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/oauth2 v0.29.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
)

replace github.com/guilhem/token-renewer => ../../
