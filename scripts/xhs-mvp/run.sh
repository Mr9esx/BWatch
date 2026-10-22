#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
VENV_DIR="$ROOT_DIR/.tools/xhs-mvp/venv"
SPIDER_DIR="$ROOT_DIR/.tools/xhs-mvp/spider-xhs"
SPIDER_REV="71d077dd39618dfbafb86130257adf97b68fe3af"
PYTHON_BIN=""
for candidate in python3.12 python3.11 python3.10; do
  if command -v "$candidate" >/dev/null 2>&1; then
    PYTHON_BIN=$(command -v "$candidate")
    break
  fi
done
if [ -z "$PYTHON_BIN" ]; then
  echo "需要 Python 3.10 或更新版本（当前 xhs-reader 依赖要求 3.10+）。" >&2
  exit 1
fi
if ! command -v node >/dev/null 2>&1; then
  echo "Spider_XHS 的签名运行环境需要 Node.js，请安装 Node.js 后重试。" >&2
  exit 1
fi

if [ ! -x "$VENV_DIR/bin/python" ]; then
  mkdir -p "$(dirname "$VENV_DIR")"
  "$PYTHON_BIN" -m venv "$VENV_DIR"
fi
"$VENV_DIR/bin/python" -m pip install --disable-pip-version-check -r "$ROOT_DIR/scripts/xhs-mvp/requirements.txt"
if [ ! -d "$SPIDER_DIR/.git" ]; then
  mkdir -p "$(dirname "$SPIDER_DIR")"
  git clone --filter=blob:none https://github.com/cv-cat/Spider_XHS.git "$SPIDER_DIR"
fi
if ! git -C "$SPIDER_DIR" rev-parse --verify "$SPIDER_REV^{commit}" >/dev/null 2>&1; then
  git -C "$SPIDER_DIR" fetch --quiet origin "$SPIDER_REV"
fi
git -C "$SPIDER_DIR" checkout --quiet --detach "$SPIDER_REV"
cd "$ROOT_DIR"
exec "$VENV_DIR/bin/python" "$ROOT_DIR/scripts/xhs-mvp/server.py"
