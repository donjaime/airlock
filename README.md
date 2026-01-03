# airlock

A lightweight CLI and set of credential management patterns to create **project-scoped, persistent container sandboxes** for local development — isolating your system from untrusted code, supply-chain attacks, and agent-driven automation.

`airlock` was inspired by the ease of use and developer convenience of **Fedora Toolbx (aka Toolbox)** for mutable, local dev workflows. But with some additional asks that Toolbox didn't quite provide. Specifically:
- **Container isolation** to limit the damage of 3rd part dependency supply-chain attacks in npm, pip, etc... (eg. malicious pre or post install scripts) - as well as create a safer sandbox for agentic tools to operate closer to YOLO mode
- **Surgical Persistent state**: project-scoped HOME + installation cache
- **Opinionated patterns for identity and credential management**: project-scoped or shared secrets, ssh and gpg credentials - again to limit what can be done inside the container, but also make it convenient to give AI agents access to things in an auditable way
- **Version controllable project configuration** to make it easy to have standard environments that can be shared

In addition to being **human friendly** (just `enter` container environments), I also tried to carry forward some things I liked. Like 
**Podman-rootless first** (though Docker is supported) to ensure containers don't have host sudo. And to be **agent-agnostic**. You can use any CLI agent

## How it works
`airlock` is a CLI tool for starting and entering container dev environments, and some workflow patterns around managing identities and credentials and supplying them into these environments.

Airlock separates **project home state** from **build/runtime caches**, while still maximizing compatibility with common developer tools.

### Host layout (project-scoped)

```
<Your Airlocked Project Root>/
  ...<Project Source Files and Folders>... # The stuff you want to work on
  
  airlock.yaml    # Version controlled project configuraton bootstrapped by `airlock init`
  Containerfile   # Custom container image to bootstrap your dev environment
 
  .gitignore      # Make sure to ignore `.airlock/` (which `airlock init` does automatically)
  .airlock/       # NOT version controlled
    home/         # persistent project home (dotfiles, config, symlinked identities)
    cache/        # persistent but disposable caches (npm, pip, go, etc.)
    airlock.local.yaml # Local-only environment vars and config. Not versioned.
```

Everything in `.airlock/` is **local-only**, not meant to be committed to version control, and is **masked** so it is inaccessible from within the container's workspace.

> If you want to build a project or repo that is not "airlock aware", you can simply have one level of folder nesting where the airlock project root is above your project - treating the other project as a subproject. You are free to use git submodules or just symlinking things into place.  


---


## Commands

- `airlock init`  
  Creates `airlock.yaml` (if missing), ensures `.airlock/` state dirs, and updates `.gitignore`.

- `airlock up`  
  Builds container image (if configured) + creates container + ensures state dirs exist.

- `airlock enter`  
  Enters the container with `bash -l`.

- `airlock exec -- <cmd...>`  
  Runs a command inside the container.

- `airlock down [name]`  
  Stops and removes the container (keeps `.airlock` state dirs). If `name` is omitted, it downs the container for the current project.

- `airlock list`  
  Lists all airlock containers.

- `airlock info`  
  Prints detected engine, paths, and config.

- `airlock version`  
  Prints version.

- `airlock help`  
  Prints this usage information.

-----

## What goes where

```
Host                      Container
------------------------  ------------------------
.airlock/home         →  /home/username         # this is the container user’s `$HOME`
.airlock/cache        →  /home/username/.cache  # the conventional XDG cache location
./                    →  /workspace             # project workdir (excluding .airlock/)
```

> **Note:** The `.airlock/` folder is explicitly masked within the container. It is not accessible from `/workspace` even though it exists in the host project root. This ensures that tools running in the sandbox cannot accidentally modify or leak its own configuration and state.



### `.airlock/home`

Persistent **user state**, such as:

- shell history

- dotfiles

- tool configuration

- symlinked identity files (SSH, AWS config, etc.)


You should treat this like a project-scoped `$HOME`.

### `.airlock/cache` (project cache)

Persistent but **disposable** data, such as:

- language package caches

- build artifacts

- dependency downloads


You should feel safe deleting this at any time to reclaim space or fix cache issues.


> **Recommended:** If you want to clear caches, delete **`.airlock/cache`**, not `.airlock/home/.cache`.

> **Note:** Seeing `.airlock/home/.cache` does **not** mean cache data is being stored inside your home directory. `.cache` is just a mount point. The file live in the cache folder.

## Project Configuration in `airlock.yaml` 

`airlock.yaml` is a small, project-scoped config file that tells Airlock:

* which container image to run (or how to build it),
* what to mount into the sandbox,
* what home/cache directories to use (defaults to `.airlock/home` and `.airlock/cache`),
* what command to run when entering the sandbox.

Airlock will **create and persist** project state under `.airlock/` by default.

### Example `airlock.yaml`

```yaml
# airlock.yaml
name: myproject
version: 1
engine: podman   # or "docker" (Optional). Defaults to podman if omitted.

# The sandbox container image to run.
# You can either point at a prebuilt image OR provide a build section in place of image.
# image: ghcr.io/your-org/airlock-dev:latest

# If build is set, Airlock will build and tag an image for this project.
build:
  context: ./env
  containerfile: ./env/Containerfile
  tag: airlock-myproject:dev

# Project-scoped persistent directories (defaults shown).
# These paths are on the host, relative to the repo root.
home: .airlock/home
cache: .airlock/cache

# To reuse across projects, point these at shared host paths, e.g.:
# home: ~/.local/share/airlock/home
# cache: ~/.local/share/airlock/cache

# This is the folder we map from the host into the container's working directory. 
workdir: .

# Optional: Mounts bind host paths into the container.
mounts:
  - source: ../test_data
    target: /test_data
    mode: rw

  # Optional: share a host-level package cache (speeds up installs).
  # - source: ~/.cache/pip
  #   target: /host-cache/pip
  #   mode: rw

ports:
  - host: 3000
    container: 3000
  - host: 54321
    container: 5432

# Environment variables to set inside the container.
env:
  # Standard: keep tools pointed at the mounted workspace.
  WORKSPACE: /workspace

  # Example: make git use the workspace by default.
  # GIT_WORK_TREE: /workspace
  
  # Additional env vars can be passed in via .airlock/airlock.local.yaml
  # You can pass local-only secrets via that file
```


---

## What each field means

### `name`

The name of the project. This is used to tag the built image and name the containers.

* Defaults to the name of the directory containing `airlock.yaml`.

### `version`

A config version for forward compatibility.

* `version: 1` is the current format.

### `engine` (optional)

The container engine to use.

* Options: `podman` (default), `docker`.

### `image`

The container image Airlock should run.

* Example: `ghcr.io/your-org/airlock-dev:latest`
* Use this when you have a standard base image for your team/org.

### `build` (optional)

If present, Airlock builds an image for this project instead of pulling `image`.

* `context`: build context directory (usually `.`)
* `containerfile`: path to Dockerfile/Containerfile (defaults to `Containerfile`)
* `tag`: local image tag to build to

Use `build` when:

* you want project-specific tooling baked into the image,
* you’re iterating on the container environment.

### `workdir` (optional)

The directory on the host that gets mapped into the container to be used as the initial working directory.

* Defaults to `.` (the directory containing the config file).

### `home` and `cache`

Host paths for **project-scoped persistence**.

* `home`: mounted as `$HOME` in the container (or otherwise used as the container user’s home).
* `cache`: a persistent cache directory intended for package managers and build caches.

Defaults:

* `home: .airlock/home`
* `cache: .airlock/cache`

You can point these at a shared location if you *want* cross-project reuse, e.g.:

```yaml
home: ~/.airlock/home/myproject
cache: ~/.airlock/cache/myproject
```

### `mounts`

A list of explicit host→container mounts.

Each mount has:

* `source`: path on the host (relative to repo root is allowed)
* `target`: path inside the container
* `mode`: `rw` or `ro`


### `env`

Environment variables to set inside the container.
> For private vars see `.airlock/airlock.local.yaml`

### `ports`

The `ports` field is a list of host ↔ container port mappings.

Each entry has:

* `host`: port number on the host machine
* `container`: port number inside the container

```yaml
ports:
  - host: 3000
    container: 3000
```

You can define multiple services on the same container. For example:

```yaml
ports:
  - host: 3000
    container: 3000
  - host: 6006
    container: 6006   # Storybook
  - host: 9229
    container: 9229   # Node inspector
```

Under the hood, Airlock translates `ports` into the container runtime’s native flags (`-p host:container`).



---


## Install

### Build from source

```bash
git clone https://github.com/donjaime/airlock
cd airlock
go build -o airlock .
```

Add `airlock` to your path or move it somewhere that is already on the path eg: 
```
sudo mv airlock /usr/local/bin/
```

## Typical workflow

1. Create config + state dirs:


```bash
airlock init
```

This creates:

- `airlock.yaml` (only if missing)

- `./.airlock/home` and `./.airlock/cache` and an empty `./airlock/airlock.local.yaml`
- ensures `.gitignore` ignores `.airlock/`


Your `airlock.yaml` is typically safe to check in to version control if it only contains stable relative configuration.
It must never contain secrets directly.

If you want to have non-version controlled local configuration, you can put that in `./.airlock/airlock.local.yaml` and any properties there will merge with the default `airlock.yaml`.

> `.airlock/airlock.local.yaml` is often a convenient way to pass in local-only tokens that typically would be set as environment variables.

2. Run:

Build and run the container
```bash
airlock up
```

Then just enter it
```bash
airlock enter
```

You should land in an interactive shell inside the container at `/workspace` (or your configured workdir).


## Identities & Credentials
Airlock intentionally does **not** manage identities internally.
Instead, identities live in a **shared host directory**, and each project explicitly **symlinks only what it needs** into its project-scoped home (`.airlock/home`) *before* entering the sandbox.

This keeps the “secret materialization step” on the host, makes access easy to audit (`ls -la .airlock/home`), and avoids hidden identity managers inside the sandbox.

### Principles
- **Never symlink whole identity directories** (e.g. don’t link all of `~/.ssh`).
- Prefer **per-project** or **per-org** identities (keys/configs/tokens) over personal “everything” identities. You can generate an identity for a CLI agent (like Claude Code) and only offer that identity inside the container.
- Keep secrets **outside the repo**, and symlink them into `.airlock/home`.
- If you have secrets already set as environment variables on the host, **you can forward them into the container** with the `-e <ENV_VAR_NAME>` flag when you `enter`.
- Treat `.airlock/home` as persistent: if a tool writes tokens/caches there, they will remain until you remove them.


### Identity store location (host)

You can absolutely symlink identities surgically from your host's home dir. But to enforce some stricter separation, we recommend creating separate credentials for ssh, gpg, etc... and storing curated identities under:

```
~/.config/airlock/identities/
```

Example layout:

```
~/.config/airlock/identities/
  work-foo/
    .ssh/
      id_ed25519_work_foo
      id_ed25519_work_foo.pub
      config
      known_hosts
    .aws/
      config
      credentials
    gh_token
```

Each subdirectory represents a **coherent identity profile** (work, client, org, etc.).

---

## Linking identities into a project (recommended pattern)

Airlock mounts `.airlock/home` as `$HOME` inside the container.
Before entering, symlink **only the required files** from the shared identity store.

### Example: SSH (single key, minimal config)

```bash
mkdir -p .airlock/home/.ssh
chmod 700 .airlock/home/.ssh

ln -sf ~/.config/airlock/identities/work-foo/.ssh/id_ed25519_work_foo \
       .airlock/home/.ssh/id_ed25519_work_foo

ln -sf ~/.config/airlock/identities/work-foo/.ssh/config \
       .airlock/home/.ssh/config

ln -sf ~/.config/airlock/identities/work-foo/.ssh/known_hosts \
       .airlock/home/.ssh/known_hosts 2>/dev/null || true
```

**Do not** symlink your entire `~/.ssh` directory.

---

## Example: Git identity (project-scoped)

```bash
ln -sf ~/.config/airlock/identities/work-foo/.gitconfig \
       .airlock/home/.gitconfig
```

---

## Example: GitHub CLI token (least privilege)

```bash
mkdir -p .airlock/home/.secrets
chmod 700 .airlock/home/.secrets

ln -sf ~/.config/airlock/identities/work-foo/gh_token \
       .airlock/home/.secrets/gh_token
```

Inside the container:

```bash
export GH_TOKEN="$(cat ~/.secrets/gh_token)"
gh auth status
```

---

## Example: AWS credentials (no global `~/.aws`)

```bash
mkdir -p .airlock/home/.aws
chmod 700 .airlock/home/.aws

ln -sf ~/.config/airlock/identities/work-foo/.aws/config \
       .airlock/home/.aws/config

ln -sf ~/.config/airlock/identities/work-foo/.aws/credentials \
       .airlock/home/.aws/credentials
```

---

## Auditing identity exposure

Before entering the sandbox, you should be able to answer:

> “Exactly which identity files can this container see?”

Check with:

```bash
find .airlock/home -type l -print -exec readlink {} \;
```

If it’s too much, remove symlinks and try again.

---

## Persistence and lifecycle notes

* `.airlock/home` is **project-scoped and persistent**.
* Identities remain linked until you remove the symlinks.
* Tools may write auth caches or tokens into `$HOME`.

Best practices:

* keep identity **sources** in `~/.config/airlock/identities/`
* treat `.airlock/` as **safe to delete and recreate** (modulo recreating runtime mutations and installations)
* prefer `.airlock/cache` for tool caches when configurable


> **If it’s in `.airlock/home` (or one of the mounted folders), the container can see it.
> If it’s not, it can’t — and this includes the `.airlock/` folder itself within your project root, which is masked.**

---


## Auditing what the sandbox can see

Before entering:

```bash
find .airlock/home -maxdepth 3 -type l -print -exec readlink {} \;
```

If you accidentally linked too much, remove it:

```bash
rm .airlock/home/.ssh   # or remove specific symlinks
```

---

## Notes on persistence and safety

- `.airlock/home` is designed to persist across container rebuilds. That’s great for dev convenience, but it means:

  - tools may write auth caches or tokens into `$HOME`;

  - your symlinks remain until removed.

- For caches and tool state, prefer `.airlock/cache` (if your airlock implementation mounts it), and configure tools to store caches there when possible.

- Keep curated identity material in a shared host folder (like `~/.airlock/identities/…`) rather than sprinkling secrets around your normal `~`.


## Secrets and API tokens

**Do not commit secrets.** `.airlock/` is ignored by default.

### Recommended:

For files, symlink them somewhere into the home folder (see section above).

For **environment variables** you can either:
- add them to `.airlock/airlock.local.yaml` under `env` (see yaml section above explaining the yaml format)

- OR explicitly forward ambient environment vars into the container when you enter it.
```bash
export ANTHROPIC_API_KEY="..."
airlock enter -e "ANTHROPIC_API_KEY"
```

## Claude Code (optional)

If installed during the container build (see default Dockerfile example provided):

```bash
claude --help
```

If it isn’t installed (upstream changes happen), install manually when inside the container:

```bash
npm install -g @anthropic-ai/claude-code
```

## License
MIT


## Future Work
- **MacOS support**. Only tested on linux. 
- **IDE support**. SDKs and libraries are often needed to power LSPs and refactoring. You could configure remote SSH manually, but that's clunky. On linux you can point the IDE at the stuff downloaded by the container in cache and home. But I think that might violate some of the desired isolation we want. 
- **Identity management CLI tools**. Promote some of the patterns to CLI functions. Manual symlinking is clunky.
- 
