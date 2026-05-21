# airlock

A lightweight CLI to create **persistent container sandboxes** for local development — isolating your system from untrusted code, supply-chain attacks, and agent-driven automation.

`airlock` was inspired by **Fedora Toolbx** for mutable, local dev workflows, with additional asks:
- **Container isolation** to limit damage from supply-chain attacks (malicious npm/pip install scripts) and create a safer sandbox for agentic tools
- **Surgical persistent state**: named, reusable identity directories (`home/` + `cache/`) that survive container rebuilds
- **Zero repo invasion**: all airlock state lives in `~/.airlock/` — no files are added to your project
- **Podman-rootless first** (Docker also supported)

---

## How it works

`airlock` stores all configuration in `~/.airlock/`. To use it with any project, you run `airlock init` once in that directory. Airlock asks you which container image to use and which identity (persistent home + cache) to attach, then remembers your choice.

After that, `airlock up` starts the container and `airlock enter` drops you into a shell, with your project mounted at the container's working directory.

### Global state layout

```
~/.airlock/
  config.yaml            # Engine preference (podman/docker)
  identities/
    <name>/              # A named persistent environment, e.g. "default", "oss", "work"
      home/              # Mounted as $HOME inside the container
      cache/             # Mounted as $HOME/.cache (package manager and build caches)
  projects.yaml          # Maps project directories → (image, identity)
```

### What gets mounted

```
Host                              Container
--------------------------------  ---------------------------
~/.airlock/identities/<id>/home   →  /home/<user>       ($HOME)
~/.airlock/identities/<id>/cache  →  /home/<user>/.cache
<your project directory>          →  /workspace          (workdir)
```

The project directory is mounted read-write. Nothing from `~/.airlock/` is visible inside the container except through the `home/` and `cache/` mounts.

---

## Commands

```
airlock init                     Set up airlock for the current directory
airlock forget [name]            Remove a project from the airlock index

airlock identity list            List all named identities
airlock identity add <name>      Create a new identity (home/ + cache/ dirs)
airlock identity remove <name>   Remove an identity (--force required to delete files)

airlock up                       Build image (if Containerfile) and start the container
airlock enter                    Open an interactive shell in the container
airlock exec -- <cmd>            Run a command inside the container
airlock down [name]              Stop and remove a container (keeps identity dirs)
airlock list                     List all known projects with image, identity, and status
airlock info                     Show resolved config for the current directory
airlock version
airlock help
```

### Flags

```
-e KEY           Forward ambient host env var KEY into the container
-e KEY=VALUE     Set KEY=VALUE inside the container
-v               Verbose: print the underlying podman/docker commands
```

---

## Typical workflow

### First time on a new system

```bash
cd ~/repos/some-project
airlock init
```

Airlock prompts you for an image and an identity:

```
Setting up airlock for /home/user/repos/some-project

No previously used images on this system.
  1. Enter a different image ref...
  2. Build from a Containerfile...
Select image [1-2]: 1
Image ref: ubuntu:24.04

No identities found. Creating a default identity automatically.
Created ~/.airlock/identities/default/

Project name [some-project]:

Saved to ~/.airlock/projects.yaml
Run `airlock up` to start the container.
```

Then:

```bash
airlock up       # pull image and start container
airlock enter    # open a shell at /workspace inside the container
```

### Subsequent projects

On your second project, `airlock init` offers previously-used images as choices so you don't have to retype them.

```
Previously used images:
  1. ubuntu:24.04  (used by: some-project)
  2. Enter a different image ref...
  3. Build from a Containerfile...
Select image [1-3]: 1

Available identities:
  1. default
  2. Create a new identity...
Select identity [1-2]: 2
Identity name: oss

Created ~/.airlock/identities/oss/
```

### Multiple terminals

Each `airlock enter` opens a new shell process inside the same running container — like extra terminals on the same machine. No extra containers are created.

---

## Container images

`airlock init` accepts any image your container runtime can use:

- **Registry images**: `ubuntu:24.04`, `docker.io/golang:1.22`, `registry.fedoraproject.org/fedora-toolbox:41`
- **Locally pulled images**: any image already in your local podman/docker image store
- **Built from a Containerfile**: provide a path; airlock runs `podman build` before starting

When choosing "Build from a Containerfile", airlock stores the containerfile path and tag in `~/.airlock/projects.yaml`. The build runs automatically on `airlock up` if the image isn't already present.

---

## Identities

An identity is a named pair of host directories (`home/` and `cache/`) that get mounted into the container. They persist across container rebuilds and are shared across any projects that reference them.

### When to share vs isolate

- **Separate identity per project** — fully isolated agent state, shell history, and tool configs. Safe default.
- **Shared identity across related projects** — same dotfiles, shell history, and tool configs. Useful for worktrees or closely related repos.

### Managing identities

```bash
airlock identity list
airlock identity add work
airlock identity remove old-identity --force
```

---

## Git Worktrees & Parallel Agents

Airlock works naturally with git worktrees. Each worktree path is its own project entry in `~/.airlock/projects.yaml`, so each gets its own container. Run `airlock init` once in each worktree.

```bash
git worktree add ../myproject-feature-auth feature/auth
cd ../myproject-feature-auth
airlock init          # pick image + identity (can share with main worktree)
airlock up && airlock enter
```

To share cache across worktrees, point them at the same identity:

```bash
# In each worktree, airlock init → pick the same identity (e.g. "myproject")
airlock identity add myproject
```

---

## Forgetting a project

```bash
# From inside the project directory:
airlock forget

# By name from anywhere:
airlock forget my-old-project

# Interactive picker (when outside any known project dir):
airlock forget
```

`airlock forget` removes the project from `~/.airlock/projects.yaml` only. It does not stop or remove the container (run `airlock down` first if needed) and does not delete the identity directories.

---

## Identities & Credentials

Identities intentionally do **not** manage secrets internally. Instead, symlink only what each project needs into the identity's `home/` directory from a shared host credential store. This keeps the "secret materialization step" on the host and makes access easy to audit.

### Recommended host credential layout

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

### Linking credentials into an identity

```bash
# SSH (single key)
mkdir -p ~/.airlock/identities/work/.home/.ssh
chmod 700 ~/.airlock/identities/work/.home/.ssh
ln -sf ~/.config/airlock/identities/work-foo/.ssh/id_ed25519_work_foo \
       ~/.airlock/identities/work/home/.ssh/id_ed25519_work_foo
ln -sf ~/.config/airlock/identities/work-foo/.ssh/config \
       ~/.airlock/identities/work/home/.ssh/config

# Git identity
ln -sf ~/.config/airlock/identities/work-foo/.gitconfig \
       ~/.airlock/identities/work/home/.gitconfig
```

**Never symlink your entire `~/.ssh`.**

### Forwarding environment variables

```bash
airlock -e ANTHROPIC_API_KEY enter
airlock -e AWS_PROFILE=my-profile enter
```

### Auditing what the container can see

```bash
find ~/.airlock/identities/<name>/home -maxdepth 3 -type l -print -exec readlink {} \;
```

---

## Install

### Build from source

```bash
git clone https://github.com/donjaime/airlock
cd airlock
go build -o airlock .
sudo mv airlock /usr/local/bin/
```

### Prerequisites

Airlock requires **Podman** (preferred, rootless) or **Docker**.

#### Linux

```bash
# Fedora / RHEL
sudo dnf install podman

# Ubuntu / Debian
sudo apt install podman
```

#### macOS

**Option A: Podman Machine** (recommended)

```bash
brew install podman
podman machine init --cpus 4 --memory 4096
podman machine start
```

**Option B: Colima + Docker**

```bash
brew install colima docker
colima start
```

> **Note:** On macOS your project must be under `$HOME` (shared into the VM by default). For other paths, configure additional VM mounts.

---

## License
MIT
