---
title: Install
weight: 9
---

You can install the pre-compiled binary (in several different ways), use Docker or compile from source.

Below you can find the steps for each of them.


## Install the pre-compiled binary

### Homebrew

```bash
brew install shopware/tap/shopware-cli
```

### Debian/Ubuntu — APT based Linux

```bash
curl -1sLf \
  'https://dl.cloudsmith.io/public/friendsofshopware/stable/setup.deb.sh' \
  | sudo -E bash
sudo apt install shopware-cli
```

### Fedora/CentOS/SUSE/RedHat — YUM based Linux

```bash
curl -1sLf \
  'https://dl.cloudsmith.io/public/friendsofshopware/stable/setup.rpm.sh' \
  | sudo -E bash
sudo dnf install shopware-cli
```

### Archlinux User Repository (AUR)

```bash
yay -S shopware-cli-bin
```

### Manually: deb,rpm apt packages

Download the .deb, .rpm or .apk packages from the [releases](https://github.com/shopware/shopware-cli/releases/) page and install them with the appropriate tools.

### Nix

```shell
nix profile install nixpkgs#shopware-cli
```

or directly from the FriendsOfShopware repository (more up to date)

```shell
nix profile install github:FriendsOfShopware/nur-packages#shopware-cli
```

### Devenv

Update `devenv.yaml` with a new input:

```yaml
inputs:
  nixpkgs:
    url: github:NixOS/nixpkgs/nixpkgs-unstable
  froshpkgs:
    url: github:FriendsOfShopware/nur-packages
    inputs:
      nixpkgs:
        follows: "nixpkgs"
```

and then you can use the new input in the `devenv.nix` file. Don't forget to add the `inputs` argument, to the first line.


```nix
{ pkgs, inputs, ... }: {
  packages = [
    inputs.froshpkgs.packages.${pkgs.system}.shopware-cli
  ];
}
```

### GitHub Codespaces

```json
{
    "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
    "features": {
        "ghcr.io/shyim/devcontainers-features/shopware-cli:latest": {}
    }
}
```

### GitHub Action

using Shopware CLI Action

```yaml
- name: Install shopware-cli
  uses: shopware/shopware-cli-action@v1
```

### Gitlab CI

```yaml
build:
  stage: build
  image:
    name: shopware/shopware-cli:latest
    entrypoint: [ "/bin/sh", "-c" ]
  script:
    - shopware-cli --version
```

### ddev

Add a file `.ddev/web-build/Dockerfile.shopware-cli`

```Dockerfile
# .ddev/web-build/Dockerfile.shopware-cli
COPY --from=shopware/shopware-cli:bin /shopware-cli /usr/local/bin/shopware-cli
```

### Docker Image

Add the following line to your docker image to copy the binary into your image. 

```Dockerfile
# Dockerfile
COPY --from=shopware/shopware-cli:bin /shopware-cli /usr/local/bin/shopware-cli
```

## manually

Download the pre-compiled binaries from the [releases](https://github.com/shopware/shopware-cli/releases/) page and copy them to the desired location.

## Running with Docker

You can also use it within a Docker container. To do that, you'll need to execute something more-or-less like the examples below.

Registries:

- [ghcr.io/friendsofshopware/shopware-cli](https://github.com/shopware/shopware-cli/pkgs/container/shopware-cli)

Example usage:

Builds assets of an extension

```
docker run \
    --rm \
    -v $(pwd):$(pwd) \
    -w $(pwd) \
    -u $(id -u) \
    ghcr.io/friendsofshopware/shopware-cli \
    extension build FroshPlatformAdminer
```

You can verify the image with cosign:

```
cosign verify ghcr.io/friendsofshopware/shopware-cli \
  --certificate-identity 'https://github.com/shopware/shopware-cli/.github/workflows/release.yml@refs/tags/0.1.69' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
```

Hint: You have to adjust the version inside the `certificate-identity`

## Compiling from source

If you just want to build from source for whatever reason, follow these steps:

### clone:

```
git clone https://github.com/shopware/shopware-cli
cd shopware-cli
```

### get the dependencies:

```
go mod tidy
```

### build:

```
go build -o shopware-cli .
```

### verify it works:

```
./shopware-cli --version
```
