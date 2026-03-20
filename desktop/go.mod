module github.com/liamawhite/ng/desktop

go 1.25.7

require (
	github.com/liamawhite/ng/api/golang v0.0.0-00010101000000-000000000000
	github.com/liamawhite/ng/backend v0.0.0-00010101000000-000000000000
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3
	github.com/wailsapp/wails/v2 v2.10.1
	google.golang.org/grpc v1.79.3
)

replace (
	github.com/liamawhite/ng/api/golang => ../api/golang
	github.com/liamawhite/ng/backend => ../backend
)
