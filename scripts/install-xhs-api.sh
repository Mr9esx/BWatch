#!/bin/sh
set -eu

PROJECT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME_DIR=${BILIFETCH_XHS_RUNTIME:-"$PROJECT_DIR/.tools/xhs-api"}
SPIDER_REV=71d077dd39618dfbafb86130257adf97b68fe3af
PYTHON_BIN=${PYTHON_BIN:-python3}
command -v node >/dev/null
mkdir -p "$RUNTIME_DIR"
if [ ! -x "$RUNTIME_DIR/venv/bin/python" ]; then
    "$PYTHON_BIN" -m venv "$RUNTIME_DIR/venv"
fi
"$RUNTIME_DIR/venv/bin/python" -m pip install --disable-pip-version-check -r "$PROJECT_DIR/scripts/xhs-api-requirements.txt"
if [ ! -d "$RUNTIME_DIR/spider-xhs/.git" ]; then
    git clone --filter=blob:none --no-checkout https://github.com/cv-cat/Spider_XHS.git "$RUNTIME_DIR/spider-xhs"
fi
git -C "$RUNTIME_DIR/spider-xhs" checkout --quiet --detach "$SPIDER_REV"
npm --prefix "$RUNTIME_DIR/spider-xhs" ci --ignore-scripts --no-audit --no-fund
echo "小红书 API 运行环境已安装。"
