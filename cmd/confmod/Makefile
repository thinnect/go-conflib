# Makefile for embedding build info into the executable

BUILD_DATE = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
BUILD_DISTRO = $(shell lsb_release -sd)

all:
	go build -o confmod -ldflags "-X main.ApplicationBuildDate $(BUILD_DATE) -X main.ApplicationBuildDistro '$(BUILD_DISTRO)'"
