module honnef.co/go/wgpu-demo

go 1.23

require (
	honnef.co/go/curve v0.0.0-20240713195357-1e39b7d8d78a
	honnef.co/go/jello v0.0.0
	honnef.co/go/libwayland v0.0.0
	honnef.co/go/wgpu v0.0.0-20240719115612-5d243632325b
)

require (
	golang.org/x/exp v0.0.0-20240613232115-7f521ea00fb8 // indirect
	honnef.co/go/safeish v0.0.0-20240708173349-a249fc03b5f3 // indirect
	honnef.co/go/wgpu-darwin-amd64 v0.1904.1 // indirect
	honnef.co/go/wgpu-darwin-arm64 v0.1904.1 // indirect
	honnef.co/go/wgpu-linux-amd64 v0.1904.1 // indirect
	honnef.co/go/wgpu-linux-arm64 v0.1904.1 // indirect
	honnef.co/go/wgpu-windows-386 v0.1904.1 // indirect
	honnef.co/go/wgpu-windows-amd64 v0.1904.1 // indirect
)

replace honnef.co/go/libwayland => ../libwayland

replace honnef.co/go/jello => ../jello
