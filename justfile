project  := "vkg"
registry := "ghcr.io"
owner    := "nugget"
image    := registry / owner / "vanitykeygen"

platforms := "linux/amd64,linux/arm64"

version  := `git describe --always --long --tags --dirty 2>/dev/null || echo "dev"`
revision := `git rev-parse HEAD 2>/dev/null || echo "unknown"`
created  := `date -u +"%Y-%m-%dT%H:%M:%SZ"`

platform := `uname -s`
arch     := `uname -m`
suffix   := platform + "-" + arch
build_dir := justfile_directory() / "build"
binary   := build_dir / "vkg-" + suffix

# List available recipes
default:
    @just --list

# Show build metadata
info:
    @echo "Version:  {{ version }}"
    @echo "Revision: {{ revision }}"
    @echo "Image:    {{ image }}"
    @echo "Binary:   {{ binary }}"

# Run all tests
test:
    go test ./...

# Run go vet
vet:
    go vet ./...

# Build native binary
build:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{ build_dir }}
    cd cmd/vkg && go build -ldflags="-X 'main.gitVersion={{ version }}'" -o {{ binary }}

# Build static binary (CGO_ENABLED=0)
build-static:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{ build_dir }}
    cd cmd/vkg && CGO_ENABLED=0 go build -ldflags="-X 'main.gitVersion={{ version }}'" -o {{ justfile_directory() }}/vkg-static-build .

# Run the server locally
run-server: build
    {{ binary }} server

# Run a client locally
run-client: build
    {{ binary }} client

# Clean build artifacts
clean:
    rm -rf {{ build_dir }}
    rm -f vkg-static-build

# Build and push multi-arch container to ghcr.io
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
