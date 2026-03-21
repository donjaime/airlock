# airlock

A lightweight CLI and set of credential management patterns to create **project-scoped, persistent container sandboxes** for local development — isolating your system from untrusted code, supply-chain attacks, and agent-driven automation.

`airlock` was inspired by the ease of use and developer convenience of **Fedora Toolbx (aka Toolbox)** for mutable, local dev workflows. But with some additional asks that Toolbox didn't quite provide. Specifically:
- **Container isolation** to limit the damage of 3rd party dependency supply-chain attacks in npm, pip, etc... (eg. malicious pre or post install scripts) - as well as create a safer sandbox for agentic tools to operate closer to YOLO mode
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

- `airlock init [name]`  
  Creates `airlock.yaml`, `Containerfile`, ensures `.airlock/` state dirs, and updates `.gitignore`. Optionally takes a project `name`.

- `airlock up`  
  Builds container image (if configured) + creates container + ensures state dirs exist.

- `airlock enter`  
  Enters the container with `bash -l`.

- `airlock exec -- <cmd...>`  
  Runs a command inside the container.

- `airlock down [name]`  
  Stops and removes the container (keeps `.airlock` state dirs). If `name` is omitted, it downs the container for the current project.

- `airlock list`
  Lists all running airlock containers. Works from any directory (no config needed).

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

If present, the container image Airlock should run. Examples shown make use of `build` instead for custom container.

* Example: `ghcr.io/your-org/airlock-dev:latest`
* Use this when you have a standard base image for your team/org.

### `build`

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

### Prerequisites

Airlock needs a container runtime. It supports **Podman** (preferred) and **Docker**.

#### Linux

```bash
# Fedora / RHEL
sudo dnf install podman

# Ubuntu / Debian
sudo apt install podman
```

Podman runs rootless out of the box on Linux. No VM or daemon required.

#### macOS

On macOS, containers run inside a lightweight Linux VM. You have several options:

**Option A: Podman Machine** (recommended)

```bash
brew install podman
podman machine init --cpus 4 --memory 4096
podman machine start
```

**Option B: Colima + Podman**

```bash
brew install colima podman
colima start --runtime podman --mount-type virtiofs
```

`virtiofs` is recommended over `sshfs` for better performance with high-inode workloads like `node_modules`.

**Option C: Colima + Docker**

```bash
brew install colima docker
colima start
```

If using Docker, set `engine: docker` in your `airlock.yaml` (or remove the `engine` line to let airlock auto-detect).

> **Note:** Your project must be under `$HOME` (which is shared into the VM by default). For projects on external drives or other paths, configure additional mounts when starting the VM (e.g., `colima start --mount /path:w` or `podman machine init --volume /path`).

#### macOS VM lifecycle

The VM is a background service. Start it once and forget about it. Airlock handles the rest:

- **Switching projects**: Each project has its own container (`airlock-<name>`). Multiple containers coexist in the same VM. Just `cd` between projects and `airlock up`.
- **VM restart** (e.g., after a reboot): Your containers are preserved. `airlock up` detects the stopped container and restarts it — no rebuild needed.
- **VM deletion** (`colima delete` / `podman machine rm`): Images and containers are destroyed, but `.airlock/home` and `.airlock/cache` on your Mac are preserved. Next `airlock up` rebuilds from the Containerfile.

## Typical workflow

```bash
airlock init [project-name]   # create config, Containerfile, .airlock/ dirs, .gitignore
airlock up                    # build image (if needed) and start container
airlock enter                 # open an interactive shell inside the container
```

You land in a shell at `/workspace` (or your configured workdir). Your `airlock.yaml` is safe to commit; secrets go in `.airlock/airlock.local.yaml` (which merges into the main config and is gitignored).

### Multiple terminals

Each `airlock enter` spawns a new shell process inside the same running container -- like opening another terminal on the same machine. Run it from as many terminals as you want. No extra containers needed.

---

## Git Worktrees & Parallel Agents

Airlock pairs naturally with **git worktrees** to run parallel sandboxed agents or work on multiple branches simultaneously. Each worktree gets its own container and workspace mount, while optionally sharing cache and home directories.

### How it works

Git and IDE operations stay on the **host**. The container handles **build tooling, compiling, and coding agents**. Each worktree is just a directory of source files from the container's perspective.

When `name` is omitted from `airlock.yaml`, Airlock defaults to the **directory basename**. Since each worktree lives in its own directory, every worktree automatically gets a unique container:

```
~/src/myproject/              → airlock-myproject
~/src/myproject-feature-auth/ → airlock-myproject-feature-auth
~/src/myproject-fix-parser/   → airlock-myproject-fix-parser
```

### Setup

**1. Initialize the main repo without an explicit name**

```bash
cd ~/src/myproject
airlock init            # name defaults to directory basename
git add airlock.yaml Containerfile .gitignore
git commit -m "Add airlock config"
```

> If your `airlock.yaml` has an explicit `name:`, remove it or override per-worktree via `.airlock/airlock.local.yaml`.

**2. Create worktrees and bring them up**

```bash
git worktree add ../myproject-feature-auth feature/auth
git worktree add ../myproject-fix-parser   fix/parser

# Terminal 1
cd ~/src/myproject-feature-auth && airlock up && airlock enter

# Terminal 2
cd ~/src/myproject-fix-parser && airlock up && airlock enter
```

Multiple `airlock enter` calls against the same worktree open additional shell sessions in the same container, just like opening more terminals.

### Sharing cache across worktrees

By default, each worktree gets its own cache (`./.airlock/cache`), meaning every worktree re-downloads dependencies. To share the **package manager download cache**, set an absolute path in `airlock.yaml`:

```yaml
cache: ~/.local/share/airlock/myproject/cache
```

This is safe because build caches (Go, npm, pip, cargo) are content-addressed with file locking -- concurrent containers building different branches write different cache keys and deduplicate shared dependencies.

> **Note:** This shares the download cache, not `node_modules` or build outputs. Those live in the workspace, which is always per-worktree.

### Sharing home across worktrees

Home can also be shared to get a single-machine feel (same dotfiles, shell history, tool configs):

```yaml
# .airlock/airlock.local.yaml
home: ~/.local/share/airlock/myproject/home
cache: ~/.local/share/airlock/myproject/cache
```

This behaves like having multiple terminals open on the same machine with direnv switching between project directories. The workspace is isolated per-worktree; everything else is shared.

Keep home **per-worktree** (the default) if you want fully isolated agent state, or **shared** if you prefer the multi-terminal feel.

### Managing containers

```bash
airlock list                        # works from any directory
airlock down myproject-feature-auth # stops and removes a specific container
```

### Tips

- **Directory names become container names.** Pick short names for worktrees.
- **`.airlock/` is per-worktree and gitignored.** Deleting a worktree deletes its `.airlock/` state.
- **Containers survive worktree deletion.** Run `airlock down` before removing a worktree, or clean up with `airlock down <name>`.
- **Shared Containerfile.** Checked into git, shared across worktrees. Feature branches that modify it build a different image (tagged uniquely since the name differs).

---

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

## Persistence and safety

`.airlock/home` persists across container rebuilds. Tools may write auth caches or tokens into `$HOME`, and symlinks remain until removed.

> **If it’s in `.airlock/home` (or one of the mounted folders), the container can see it. If it’s not, it can’t — including the `.airlock/` folder itself, which is masked from within the container.**

**Audit what the sandbox can see:**

```bash
find .airlock/home -maxdepth 3 -type l -print -exec readlink {} \;
```

**Best practices:**
- Keep identity sources in `~/.config/airlock/identities/`
- Treat `.airlock/` as safe to delete and recreate
- Prefer `.airlock/cache` for tool caches when configurable


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

If installed during the container build (see default Containerfile example provided):

```bash
claude --help
```

If it isn’t installed (upstream changes happen), install manually when inside the container:

```bash
npm install -g @anthropic-ai/claude-code
```

## License
MIT
