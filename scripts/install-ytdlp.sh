#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
PROJECT_DIR=$(CDPATH= cd "$SCRIPT_DIR/.." && pwd)
VENV_DIR="$PROJECT_DIR/.tools/venv"
PYTHON_BIN=${PYTHON:-python3}

if [ "$(uname -s)" = Darwin ]; then
  if ! command -v brew >/dev/null 2>&1; then
    printf 'macOS 安装需要 Homebrew；请先安装 Homebrew，再重新运行此脚本。\n' >&2
    exit 1
  fi
  BREW_PREFIX=$(brew --prefix)
  PATH="$BREW_PREFIX/bin:$BREW_PREFIX/sbin:$PATH"
  export PATH
  brew install python ffmpeg
fi

if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  printf '未找到 %s，请先安装 Python 3.10 或更高版本。\n' "$PYTHON_BIN" >&2
  exit 1
fi
if ! "$PYTHON_BIN" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)'; then
  printf '%s 版本过低，请使用 Python 3.10 或更高版本。\n' "$PYTHON_BIN" >&2
  exit 1
fi

"$PYTHON_BIN" -m venv "$VENV_DIR"
"$VENV_DIR/bin/python" -m pip install --disable-pip-version-check -r "$PROJECT_DIR/requirements/yt-dlp.txt"
"$VENV_DIR/bin/yt-dlp" --version
printf 'yt-dlp 已安装到 %s\n' "$VENV_DIR/bin/yt-dlp"
