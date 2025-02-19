# syntax=docker/dockerfile:1.4

# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build the manager binary
# Run this with docker build --build-arg builder_image=<golang:x.y.z>
ARG builder_image
ARG deployment_base_image
ARG deployment_base_image_tag
ARG goprivate

FROM ${builder_image} as builder
WORKDIR /workspace

# Run this with docker build --build-arg goproxy=$(go env GOPROXY) to override the goproxy
ARG goproxy=https://proxy.golang.org
# Run this with docker build --build-arg package=./controlplane/kubeadm or --build-arg package=./bootstrap/kubeadm
ENV GOPROXY=$goproxy
ENV GOPRIVATE=$goprivate

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=secret,id=netrc,required=false,target=/root/.netrc \
    --mount=type=cache,target=/go/pkg/mod \
  go mod download

# Copy the sources
COPY ./ ./

# Build
ARG path=cmd/main.go
ARG ARCH
ARG ldflags

# Do not force rebuild of up-to-date packages (do not use -a)
RUN --mount=type=cache,target=/go/pkg/mod \
  CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} \
  go build -ldflags "${ldflags} -extldflags '-static'" \
  -o manager ${path}

# Production image
FROM ${deployment_base_image}:${deployment_base_image_tag}
WORKDIR /
COPY --from=builder /workspace/manager .
# Use uid of nonroot user (65532) because kubernetes expects numeric user when applying pod security policies
USER 65532
ENTRYPOINT ["/manager"]
