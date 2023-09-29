module github.com/brianmcgee/nvix

go 1.20

require (
	code.tvl.fyi/tvix/castore/protos v0.0.0-20230922125121-72355662d742
	code.tvl.fyi/tvix/store/protos v0.0.0-20230922173605-a8f079a8704f
	github.com/alecthomas/kong v0.8.0
	github.com/charmbracelet/log v0.2.4
	github.com/golang/protobuf v1.5.3
	github.com/jotfs/fastcdc-go v0.2.0
	github.com/juju/errors v1.0.0
	github.com/nats-io/nats-server/v2 v2.9.22
	github.com/nats-io/nats.go v1.30.2
	github.com/stretchr/testify v1.8.4
	github.com/ztrue/shutdown v0.1.1
	golang.org/x/sync v0.3.0
	google.golang.org/grpc v1.58.2
	google.golang.org/protobuf v1.31.0
	lukechampine.com/blake3 v1.2.1
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/lipgloss v0.8.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/klauspost/compress v1.17.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.15.2 // indirect
	github.com/nats-io/jwt/v2 v2.5.0 // indirect
	github.com/nats-io/nkeys v0.4.5 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/nix-community/go-nix v0.0.0-20230825195510-c72199eca18e // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	golang.org/x/crypto v0.13.0 // indirect
	golang.org/x/net v0.15.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230920204549-e6e6cdab5c13 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

//replace code.tvl.fyi/tvix/store/protos => /home/brian/Development/tvl.fyi/depot/tvix/store/protos
