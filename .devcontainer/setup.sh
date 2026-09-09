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
fi

sudo apt-get update
sudo apt-get install -y ffmpeg

sudo sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

go install honnef.co/go/tools/cmd/staticcheck@latest
go install golang.org/x/tools/cmd/deadcode@latest