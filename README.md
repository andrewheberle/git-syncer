# git-syncer

This is similar to other tools like
[git-sync](https://github.com/kubernetes/git-sync) but much more narrow in scope
and less generic as far as:

1. HTTP/SSH credentials for Git can be optionally retrieved from a Consul KV
	store (no other method is provided for handling credentials at this time)
2. A reload command can be specified (no webhooks or signals at this time)
3. The reload command will only be run if the filter (a regexp) matches the
	changed files

## Status

This is very early days so there are likely a lot of bugs here.

At this time, testing has focused exclusively on HTTP remotes, so despite the options
being available to support SSH remotes, this is totally untested.

## Usage

The following example will clone the specified repository from GitHub to
the provided path and perform a pull evert 5-minutes.

If any changes affect a file named `config.yaml` then `systemctl reload foo`
will be executed.

```sh
git-syncer \
	--git.url https://github.com/user/repo.git \
	--git.workdir /path/to/workdir \
	--change.filter '^config.yaml$' \
	--change.command "systemctl reload foo" \
	--interval 5m
```

After a repository has been cloned locally the `--git.url` option is no longer
required.

### Consul Support

It is possible to retrieve credentials from a Consul KV service as follows:

```sh
git-syncer \
	--git.workdir /path/to/workdir \
	--git.http.auth bearer \
	--consul.addr https://consul.example.com \
	--consul.git.password GIT_BEARER_TOKEN
```

As no `--git.url` option has been provided abive, the location specified by
`--git.workdir` must already have been cloned.

Based on the provided options above the command will perform a single pull from
the `origin` remote using HTTP Bearer based authentication with the Bearer
token being retrieved from the key named `GIT_BEARER_TOKEN` in the Consul KV
store `https://consul.example.com`

## Installation

### GitHub Releases

```sh
GIT_SYNCER_VER=0.2.0
wget https://github.com/andrewheberle/git-syncer/releases/download/v${GIT_SYNCER_VER}/git-syncer_linux_x86_64.tar.gz
tar -zxf git-syncer_linux_x86_64.tar.gz
```

#### Verifying a Release

Releases can be verified with `cosign` using the signed checksums file as
follows:

```sh
GIT_SYNCER_VER=0.2.0
# Download checksums and cosign signature data
wget https://github.com/andrewheberle/git-syncer/releases/download/v${GIT_SYNCER_VER}/checksums.txt
wget https://github.com/andrewheberle/git-syncer/releases/download/v${GIT_SYNCER_VER}/checksums.txt.sigstore.json
# Verify signature of checksums.txt file
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json --certificate-identity=https://github.com/andrewheberle/git-syncer/.github/workflows/release.yml@refs/tags/v${GIT_SYNCER_VER} --certificate-oidc-issuer=https://token.actions.githubusercontent.com
# Download release
wget https://github.com/andrewheberle/git-syncer/releases/download/v${GIT_SYNCER_VER}/git-syncer_linux_x86_64.tar.gz
# Verify SHA256 checksum of release
sha256sum -c --ignore-missing checksums.txt
tar -zxf git-syncer_linux_x86_64.tar.gz
```

### APT Package Repository

Packages are available for Debian and Ubuntu and can be installed as follows:

```sh
curl -fsSL https://packages.hebs.net.au/git-syncer/pubkey.gpg | sudo gpg --dearmor -o /usr/share/keyrings/git-syncer.gpg
echo "deb [signed-by=/usr/share/keyrings/git-syncer.gpg] https://packages.hebs.net.au/git-syncer stable main" | sudo tee /etc/apt/sources.list.d/git-syncer.list
sudo apt-get update
sudo apt-get install git-syncer
```

## Command Line Options

The complete command line options are below:

| Option | Type | Default | Description |
|--|--|--|--|
| `--change.command` | string |  | Command to run on changes |
| `--change.filter` | string | `.*` | Filter to limit changes to trigger the configured command (if any) |
| `--consul.addr` | string |  | Address of Consul KV store |
| `--consul.ca` | string |  | CA to verify connection to Consul |
| `--consul.cert` | string |  | Client certificate for Consul authentication |
| `--consul.git.password` | string |  | Consul key that holds HTTP password/token or SSH key |
| `--consul.git.user`| string |  | Consul key that holds HTTP username (used for basic auth only) |
| `--consul.key` | string |  | Client key for Consul authentication |
| `--debug` | bool | `false` | Enable debug logging |
| `--git.http.auth` | string | `basic` | HTTP Authentication type for git operations (basic or bearer) |
| `--git.ssh.knownhosts`  | string |  | Path to known_hosts file to verify SSH host keys (required for private SSH remotes) |
| `--git.remote` | string | `origin` | The git remote name |
| `--git.url` | string |  | URL of git repository (only required for the initial clone) |
| `--git.workdir` | string | | Directory for the git repository |
| `--interval` | duration |  | Refresh interval |
| `--version` |  |  | Show version and exit |
