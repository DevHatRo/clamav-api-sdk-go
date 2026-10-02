module github.com/DevHatRo/clamav-api-sdk-go/grpc

go 1.27

require (
	github.com/DevHatRo/clamav-api-sdk-go v0.0.0-20260217145920-468bda96f466
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
)

replace github.com/DevHatRo/clamav-api-sdk-go => ../
