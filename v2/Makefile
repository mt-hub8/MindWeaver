.PHONY: verify verify-standalone test build

GO ?= go

verify:
	MW_GO="$(GO)" ./scripts/ci.sh

verify-standalone:
	MW_GO="$(GO)" ./scripts/verify-standalone.sh

test:
	GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS='-mod=readonly -buildvcs=false' "$(GO)" test ./...

build:
	GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS='-mod=readonly -buildvcs=false' "$(GO)" build ./cmd/mindweaver
	GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS='-mod=readonly -buildvcs=false' "$(GO)" build ./cmd/mindweaver-pdf
