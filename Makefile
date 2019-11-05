BUILD_VERSION   := $(shell cat version)

.PHONY :  release

release:
ifneq ($(shell type ghr >/dev/null 2>&1;echo $$?), 0)
	@echo "Can't find ghr command, will start installation..."
	GO111MODULE=off go get -v -u github.com/tcnksm/ghr
endif
	ghr -u ehlxr -t $(GITHUB_RELEASE_TOKEN) -replace -delete --debug ${BUILD_VERSION}

# this tells 'make' to export all variables to child processes by default.
.EXPORT_ALL_VARIABLES:

GO111MODULE = on
GOPROXY = https://goproxy.cn,direct
GOSUMDB = sum.golang.google.cn
