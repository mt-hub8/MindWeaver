.PHONY: verify verify-standalone test build

GO ?= go

verify:
	MW_GO="$(GO)" ./scripts/ci.sh

verify-standalone:
	MW_GO="$(GO)" ./scripts/verify-standalone.sh

test:
	GOAMD64=v1 GOEXPERIMENT='' GOFIPS140=off GOTELEMETRY=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -trimpath -buildvcs=false' "$(GO)" mod download
	GOAMD64=v1 GOEXPERIMENT='' GOFIPS140=off GOTELEMETRY=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -trimpath -buildvcs=false' "$(GO)" mod verify
	GOAMD64=v1 GOEXPERIMENT='' GOFIPS140=off GOTELEMETRY=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -trimpath -buildvcs=false' "$(GO)" test ./...

build:
	GOAMD64=v1 GOEXPERIMENT='' GOFIPS140=off GOTELEMETRY=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -trimpath -buildvcs=false' "$(GO)" mod download
	GOAMD64=v1 GOEXPERIMENT='' GOFIPS140=off GOTELEMETRY=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -trimpath -buildvcs=false' "$(GO)" mod verify
	GOAMD64=v1 GOEXPERIMENT='' GOFIPS140=off GOTELEMETRY=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -trimpath -buildvcs=false' "$(GO)" build ./cmd/mindweaver
	GOAMD64=v1 GOEXPERIMENT='' GOFIPS140=off GOTELEMETRY=off GOTOOLCHAIN=local GOFLAGS='-mod=readonly -trimpath -buildvcs=false' "$(GO)" build ./cmd/mindweaver-pdf
