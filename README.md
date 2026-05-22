# airlock

Persistent container sandboxes for local development. Run AI coding agents, untrusted dependencies, and one-off experiments without giving them access to your home directory.

`airlock` composes three independent things — a **container image**, a **persistent identity**, and your **project directory** — into a sandboxed dev environment you can drop into with `airlock enter`. State you want to keep (shell history, auth tokens, package caches) lives in the identity. State you don't want to keep (random `npm install` postinstall scripts, an agent writing files where it shouldn't) stays in the container and gets wiped on rebuild.

```
        Image                  Identity                 Project
   ┌───────────────┐    ┌──────────────────────┐   ┌──────────────┐
   │ golang:1.25   │    │  ~/.airlock/         │   │ ~/repos/a    │
   │ + claude code │    │   identities/work/   │   │              │
   │ + opencode    │    │     home/  ← $HOME   │   │ ~/repos/b    │
   │ + your tools  │    │     cache/ ← caches  │   │              │
   │ (immutable)   │    │   (persistent)       │   │ (mounted)    │
   └───────┬───────┘    └──────────┬───────────┘   └──────┬───────┘
           │                       │                       │
           └───────────────────────┼───────────────────────┘
                                   ▼
                          ┌────────────────┐
                          │  airlock-a     │
                          │  airlock-b     │   composable containers
                          └────────────────┘
```

Any image, any identity, any project — mix and match. One image with many identities for work/personal/OSS separation. One identity shared across worktrees for the same project. The pieces are independent so you assemble what you need.

---

## Why airlock

- **AI agents need a blast radius.** Claude Code, opencode, Cursor's agent mode — they'll run any command they think is reasonable. Inside an airlock container, "reasonable" can't include reading your SSH keys or deleting `~/Documents`.
- **Supply-chain attacks.** `npm install` runs arbitrary postinstall scripts. `pip install` runs `setup.py`. On a normal dev machine those have your home directory. In airlock they don't.
- **Multiple projects without dotfile conflicts.** Different identities mean different `~/.gitconfig`, `~/.aws/credentials`, shell history, tool configs. No more accidentally committing with your work email on an OSS PR.
- **Reset without losing your stuff.** The image is reproducible. The identity is persistent. The project is wherever it is on disk. Blow away the container without losing accumulated state.
- **Zero repo invasion.** Nothing about airlock leaks into your project directory. No `.airlock/` folder, no config file, no `.gitignore` entry. Your public repos stay clean.
- **Podman-rootless first.** Default to the safer engine. Docker works too.

---

## Quick start

```bash
cd ~/repos/some-project
airlock init                    # interactive: pick image, identity, ports
airlock up                      # build (if needed) and start the container
airlock enter                   # drop into a shell at /workspace
```

From a new terminal anywhere on your system:

```bash
airlock enter some-project      # jump back in by project name
```

When you're done, `airlock down` stops the container. The identity persists. Next `airlock up` starts fresh from the image with your identity restored.

---

## How it works

`airlock` stores everything in `~/.airlock/`:

```
~/.airlock/
  config.yaml              Engine preference (podman/docker)
  projects.yaml            Map of project dirs → (image, identity, ports)
  identities/
    work/
      home/                ← mounted as $HOME inside the container
      cache/               ← mounted as $HOME/.cache
      setup.sh             ← host-side dotfile setup (optional)
      on-create.sh         ← first-boot setup inside the container (optional)
```

On `airlock up`, the container is created with three mounts:

```
Host                                 Container
────                                 ─────────
~/.airlock/identities/<id>/home  →   /home/<user>          ($HOME)
~/.airlock/identities/<id>/cache →   /home/<user>/.cache
<your project directory>          →   /workspace             (workdir)
```

Nothing else on your host is visible to processes inside the container.

```
  ┌─────────────────────────────────────────────────────────────┐
  │                         your host                            │
  │                                                              │
  │   ~/.ssh      ~/.aws       ~/Documents       ~/projects/...  │
  │   ~/.config (your real one)                                  │
  │                                                              │
  │   ┌──────────────────────────────────────────────────────┐   │
  │   │                  airlock container                    │   │
  │   │                                                       │   │
  │   │   /workspace     ←  ~/repos/myproj                   │   │
  │   │   $HOME          ←  identity home/                   │   │
  │   │   $HOME/.cache   ←  identity cache/                  │   │
  │   │                                                       │   │
  │   │   ┌────────────────────────────────────────────┐     │   │
  │   │   │  claude, opencode, npm, pip, your shell,    │     │   │
  │   │   │  anything you or an agent runs in here      │     │   │
  │   │   └────────────────────────────────────────────┘     │   │
  │   │   Can only touch the mounts above.                    │   │
  │   └──────────────────────────────────────────────────────┘   │
  └─────────────────────────────────────────────────────────────┘
```

### What persists vs. what gets wiped

| What                                       | Where                                | Survives `airlock down`?     |
|--------------------------------------------|--------------------------------------|------------------------------|
| Your code                                  | host filesystem, mounted in          | Yes — it's on the host        |
| Shell history, dotfiles, auth tokens       | `identities/<id>/home/`              | Yes                           |
| npm / pip / go / cargo caches              | `identities/<id>/cache/`             | Yes                           |
| Installed system packages, `/etc`, `/opt`  | image layer                          | Yes, until image rebuild      |
| Anything written outside those mounts      | container's writable layer           | **No — wiped on `down`**      |

That last row is the safety net. If anything inside the container writes to `/tmp`, `/var`, `/srv`, or anywhere not mounted, the change is in the ephemeral container layer and disappears on `airlock down`. Agents and installers can't quietly accumulate state outside the spots you've explicitly allowed.

---

## Commands

```
airlock init [-p host:container] Set up airlock for the current directory
airlock forget [name]            Remove a project from the airlock index

airlock identity list            List all named identities
airlock identity add <name>      Create a new identity (home/ + cache/ dirs)
airlock identity setup <name>    Run the identity's setup.sh on the host
airlock identity remove <name>   Remove an identity (--force required)

airlock up                       Build image (if needed) and start the container
airlock enter [name]             Open an interactive shell in the container
airlock exec [name] -- <cmd>     Run a command inside the container
airlock down [name]              Stop and remove a container (keeps identity)
airlock list                     List all known projects with status and path
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

### Port forwarding

Specify ports at `airlock init` time with `-p`. They're stored in `~/.airlock/projects.yaml` and passed to the container on creation.

```bash
airlock init -p 3000:3000 -p 5432:5432
airlock init -p 8080                    # bare port → 8080:8080
```

To change ports, re-run `airlock init -p <new-ports>` (stop the container first with `airlock down`). On re-init, the interactive prompts default to your existing settings — press Enter to keep them, only change what you need.

---

## Typical workflows

### First project on a new system

```bash
cd ~/repos/some-project
airlock init
```

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

### Subsequent projects

On the second project, `init` offers previously-used images so you don't retype them:

```
Previously used images:
  1. ubuntu:24.04
  2. Enter a different image ref...
  3. Build from a Containerfile...
Select image [1-3]: 1

Available identities:
  1. default
  2. Create a new identity...
Select identity [1-2]: 2
Identity name: oss
```

### Multiple terminals on one container

Each `airlock enter` opens a new shell in the same running container — like extra tabs on the same machine. No extra containers are created.

### Jumping into a container from anywhere

```bash
airlock enter some-project      # by name, from any cwd
airlock exec some-project -- go test ./...
```

---

## Example Containerfiles

The `examples/` directory has ready-to-use Containerfiles for common stacks. Copy one into your project as `Containerfile`, then `airlock init` and choose "Build from a Containerfile".

| File                                    | Stack                                                                    |
|-----------------------------------------|--------------------------------------------------------------------------|
| `Containerfile.golang`                  | Go 1.25, non-root `dev` user, `GOCACHE`/`GOMODCACHE` → XDG cache         |
| `Containerfile.typescript`              | Node 22 + TypeScript, tsx, ts-node, npm cache → XDG                      |
| `Containerfile.python-datascience`      | Python 3.12, Jupyter, pandas, numpy, scikit-learn, uv (EXPOSE 8888)      |
| `Containerfile.systems`                 | Ubuntu 24.04, clang, cmake, gdb, valgrind; optional Rust                 |
| `Containerfile.everything`              | All of the above + Claude Code + opencode — a kitchen-sink AI dev box    |

All examples:
- Create a non-root `dev` user at UID 1000 so Podman rootless UID mapping is clean
- Point package manager caches at `$HOME/.cache` (mounted from the host identity) so dependencies survive container rebuilds
- Use `$HOME` instead of hardcoded paths so the same script works with any image user

### Example: AI dev box

```bash
cp examples/Containerfile.everything ./Containerfile
airlock init -p 3000:3000 -p 8080:8080 -p 8888:8888
airlock up && airlock enter
```

Inside the container, log in to Claude Code once. The OAuth credentials land in `$HOME/.claude/`, which lives in your identity dir on the host — so subsequent containers built from the same identity are already authenticated.

```bash
claude            # first run: complete OAuth in browser; credentials persist
opencode          # same story
go test ./...     # use the rest of the toolchain
```

> Note: tools that install into `$HOME` during image build (Claude Code, nvm, rustup, etc.) get hidden at runtime by the identity home mount. The examples handle this by either (a) copying the resulting binary into `/usr/local/bin` as root, or (b) deferring installation to `on-create.sh`, which runs *after* the mount is in place. See `Containerfile.everything` for the pattern.

---

## Identities in depth

An identity is a named pair of host directories (`home/` and `cache/`) that get mounted into the container. They persist across container rebuilds and can be shared across projects.

### When to share vs. isolate

- **Separate identity per project** — fully isolated agent state, shell history, tool configs. Safest default for unrelated projects.
- **Shared identity across related projects** — same dotfiles, shell history, configs. Good for worktrees or closely related repos.

### Managing identities

```bash
airlock identity list
airlock identity add work
airlock identity setup work          # run setup.sh on the host
airlock identity remove old --force
```

### Home directory bootstrapping

On first `airlock up`, airlock seeds the identity's `home/` with the image's `/etc/skel` defaults — so you get a working shell (`.bashrc`, `.profile`, basic prompt) immediately with no configuration.

Each identity also gets two hook scripts beside `home/` and `cache/`:

```
~/.airlock/identities/<name>/
  setup.sh        ← runs on the HOST; wire up dotfiles and credentials
  on-create.sh    ← runs INSIDE the container on first creation; install tools
  home/
  cache/
```

**`setup.sh`** — host-side, run explicitly with `airlock identity setup <name>`. Receives `$IDENTITY_HOME` pointing to the identity's `home/`. Good for symlinking from a dotfiles repo or credential store. Safe to re-run.

```bash
# ~/.airlock/identities/work/setup.sh
ln -sf ~/dotfiles/.gitconfig "$IDENTITY_HOME/.gitconfig"
ln -sf ~/dotfiles/.bashrc    "$IDENTITY_HOME/.bashrc"
```

**`on-create.sh`** — runs inside the container automatically on first `airlock up`. Use `$HOME` (not hardcoded paths) so the script works with any image. Good for shell frameworks, git config, version managers.

```bash
# ~/.airlock/identities/work/on-create.sh
git config --global user.name  "Your Name"
git config --global user.email "yourname@example.com"
```

Both scripts start as commented templates. Airlock only runs `on-create.sh` if it contains at least one non-comment line. A sentinel file (`$HOME/.airlock-bootstrapped`) prevents re-running on subsequent `airlock up` calls.

---

## Git worktrees & parallel agents

Each worktree path is its own entry in `~/.airlock/projects.yaml`, so each gets its own container. Run `airlock init` once per worktree.

```bash
git worktree add ../myproj-feature-auth feature/auth
cd ../myproj-feature-auth
airlock init          # pick image + identity (can share with main worktree)
airlock up && airlock enter
```

To share caches across worktrees, point them at the same identity:

```bash
airlock identity add myproj
# In each worktree, airlock init → pick the same identity ("myproj")
```

This is also how you run multiple agents in parallel without them stepping on each other — separate worktrees, separate containers, shared cache, isolated working trees.

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

`airlock forget` removes the project from `~/.airlock/projects.yaml` only. It does not stop or remove the container (run `airlock down` first if needed) and does not delete identity directories.

---

## Identities & credentials

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
mkdir -p ~/.airlock/identities/work/home/.ssh
chmod 700 ~/.airlock/identities/work/home/.ssh
ln -sf ~/.config/airlock/identities/work-foo/.ssh/id_ed25519_work_foo \
       ~/.airlock/identities/work/home/.ssh/id_ed25519_work_foo
ln -sf ~/.config/airlock/identities/work-foo/.ssh/config \
       ~/.airlock/identities/work/home/.ssh/config

# Git identity
ln -sf ~/.config/airlock/identities/work-foo/.gitconfig \
       ~/.airlock/identities/work/home/.gitconfig
```

**Never symlink your entire `~/.ssh`.** The whole point is to expose only what this identity needs.

### Forwarding environment variables

```bash
airlock -e ANTHROPIC_API_KEY enter      # forward from host
airlock -e AWS_PROFILE=my-profile enter # set directly
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
NOTE: Not extensively tested yet on Mac! But it should work via podman or colima+docker.

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

> On macOS your project must be under `$HOME` (shared into the VM by default). For other paths, configure additional VM mounts.

---

## License

MIT
