#!/usr/bin/env python3
"""Build a 64D PT/EN Model2Vec model for MicroGraphRAG.

This script runs OFF DEVICE. The resulting model.safetensors and tokenizer.json
can be copied to $GO_POTION_HOME/BASE2M/ on the target device. go-potion will
then load them locally without downloading anything.
"""

from __future__ import annotations

import argparse
from pathlib import Path

from model2vec.distill import distill


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--teacher", default="intfloat/multilingual-e5-small")
    ap.add_argument("--out", default="models/micro-pt-en-64")
    ap.add_argument("--vocab", help="optional UTF-8 file: one vocabulary item per line")
    args = ap.parse_args()

    vocabulary = None
    if args.vocab:
        vocabulary = [
            line.strip()
            for line in Path(args.vocab).read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]

    model = distill(
        model_name=args.teacher,
        vocabulary=vocabulary,
        pca_dims=64,
    )
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    model.save_pretrained(out)
    print(f"saved 64D Model2Vec model to {out}")
    print("deploy as: $GO_POTION_HOME/BASE2M/model.safetensors + tokenizer.json")


if __name__ == "__main__":
    main()
