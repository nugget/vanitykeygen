project  := "vkg"
registry := "ghcr.io"
owner    := "nugget"
image    := registry / owner / "vanitykeygen"

platforms := "linux/amd64,linux/arm64"

version  := `git describe --always --long --tags --dirty 2>/dev/null || echo "dev"`
revision := `git rev-parse HEAD 2>/dev/null || echo "unknown"`
created  := `date -u +"%Y-%m-%dT%H:%M:%SZ"`

host_os   := `uname -s | tr '[:upper:]' '[:lower:]'`
host_arch := if `uname -m` == "x86_64" { "amd64" } else if `uname -m` == "aarch64" { "arm64" } else if `uname -m` == "arm64" { "arm64" } else { `uname -m` }

ldflags := "-s -w -X 'main.gitVersion=" + version + "'"

# List available recipes
default:
    @just --list

# Show build metadata
[group('info')]
info:
    @echo "Version:  {{ version }}"
    @echo "Revision: {{ revision }}"
    @echo "Image:    {{ image }}"

# Build a binary into dist/ (defaults to current platform, or specify OS/ARCH)
[group('build')]
build target_os=host_os target_arch=host_arch:
    @mkdir -p dist
    GOOS={{target_os}} GOARCH={{target_arch}} CGO_ENABLED=0 go build -trimpath -ldflags "{{ldflags}}" -o dist/vkg-{{target_os}}-{{target_arch}} ./cmd/vkg
    @if [ "{{target_os}}" = "darwin" ]; then codesign -f -s - dist/vkg-{{target_os}}-{{target_arch}} 2>/dev/null && echo "Signed dist/vkg-{{target_os}}-{{target_arch}}"; fi
    @echo "Built dist/vkg-{{target_os}}-{{target_arch}}"

# Build for all release targets
[group('build')]
build-all:
    just build linux amd64
    just build linux arm64
    just build darwin amd64
    just build darwin arm64

# Build and show version
[group('build')]
version: build
    dist/vkg-{{host_os}}-{{host_arch}} version

# Run all tests
[group('test')]
test:
    go test ./...

# Run go vet
[group('test')]
vet:
    go vet ./...

# Run the server locally
[group('run')]
run-server: build
    dist/vkg-{{host_os}}-{{host_arch}} server

# Run a client locally
[group('run')]
run-client: build
    dist/vkg-{{host_os}}-{{host_arch}} client

# Clean build artifacts
[group('build')]
clean:
    rm -rf dist

# Build and push multi-arch container to ghcr.io
[group('container')]
package tag=version:
    #!/usr/bin/env bash
    set -euo pipefail

    echo "Building multi-arch image: {{ image }}:{{ tag }}"
    echo "Platforms: {{ platforms }}"

    # Ensure builder exists
    builder="vkg-builder"
    if ! docker buildx inspect "$builder" &>/dev/null; then
        docker buildx create --name "$builder" --driver docker-container
    fi

    # Build and push with version tags
    docker buildx build \
        --builder "$builder" \
        --platform {{ platforms }} \
        --build-arg OCI_IMAGE_VERSION={{ tag }} \
        --label "org.opencontainers.image.created={{ created }}" \
        --label "org.opencontainers.image.revision={{ revision }}" \
        --label "org.opencontainers.image.version={{ tag }}" \
        --label "org.opencontainers.image.source=https://github.com/{{ owner }}/vanitykeygen" \
        -t {{ image }}:{{ tag }} \
        -t {{ image }}:latest \
        --push .

    echo ""
    echo "Pushed: {{ image }}:{{ tag }}"
    echo "Pushed: {{ image }}:latest"

# Login to GitHub Container Registry
[group('container')]
ghcr-login:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -f /Users/nugget/Sync/Projects/AI/Claude/identity/github_token ]; then
        cat /Users/nugget/Sync/Projects/AI/Claude/identity/github_token | docker login ghcr.io -u {{ owner }} --password-stdin
    else
        echo "Run: echo \$GITHUB_TOKEN | docker login ghcr.io -u {{ owner }} --password-stdin"
        exit 1
    fi

# Build, push, and attach image to a GitHub release
[group('container')]
release tag: (package tag)
    #!/usr/bin/env bash
    set -euo pipefail

    echo "Attaching container image reference to release {{ tag }}"

    # Create a manifest file to attach as a release asset
    manifest=$(mktemp)
    cat > "$manifest" <<MANIFEST
    {
        "image": "{{ image }}:{{ tag }}",
        "platforms": "{{ platforms }}",
        "created": "{{ created }}",
        "revision": "{{ revision }}"
    }
    MANIFEST

    # Upload as release asset if the release exists
    if gh release view "{{ tag }}" &>/dev/null; then
        gh release upload "{{ tag }}" "$manifest#container-manifest.json" --clobber
        echo "Attached manifest to release {{ tag }}"
    else
        echo "Release {{ tag }} not found. Create it with:"
        echo "  gh release create {{ tag }} --title '{{ tag }}' --generate-notes"
    fi

    rm -f "$manifest"
