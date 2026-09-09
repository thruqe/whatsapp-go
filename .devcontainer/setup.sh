#!/usr/bin/env bash
set -e

export GOTOOLCHAIN=auto

for rc in ~/.bashrc ~/.zshrc; do
  if [ -f "$rc" ] && ! grep -q "GOTOOLCHAIN=auto" "$rc"; then
    echo 'export GOTOOLCHAIN=auto' >> "$rc"
  fi
done

if [[ -n "${SSH_KEY:-}" ]]; then
  mkdir -p ~/.ssh
  printf "%s\n" "$SSH_KEY" > ~/.ssh/id_ed25519
  chmod 700 ~/.ssh
  chmod 600 ~/.ssh/id_ed25519
  ssh-keyscan -t ed25519 github.com >> ~/.ssh/known_hosts 2>/dev/null

  git config --global gpg.format ssh
  git config --global user.signingkey ~/.ssh/id_ed25519
  git config --global commit.gpgsign true
  git config --global tag.gpgSign true

  EMAIL="$(git config user.email || true)"
  if [[ -n "$EMAIL" ]]; then
    PUB_KEY="$(ssh-keygen -y -f ~/.ssh/id_ed25519)"
    echo "$EMAIL $PUB_KEY" > ~/.ssh/allowed_signers
    git config --global gpg.ssh.allowedSignersFile ~/.ssh/allowed_signers
  fi
fi

# Install ffmpeg and git-lfs via apt
sudo apt-get update
sudo apt-get install -y ffmpeg git-lfs
git lfs install

# Ensure remote points to SSH
git remote set-url origin git@github.com:ThruqeLabs/whatsrook.git 2>/dev/null || true

# Install Taskfile
sudo sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Install Go tools
go install honnef.co/go/tools/cmd/staticcheck@latest
go install golang.org/x/tools/cmd/deadcode@latest