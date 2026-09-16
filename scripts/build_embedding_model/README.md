# Build the PT/EN 64D embedder

This step is intentionally **off-device**. It distills a multilingual teacher into a static 64-dimensional Model2Vec model.

```bash
python -m venv .venv
. .venv/bin/activate
pip install -r scripts/build_embedding_model/requirements.txt
python scripts/build_embedding_model/distill.py --out models/micro-pt-en-64
```

For a technical vocabulary, create a UTF-8 file with one vocabulary item per line and pass `--vocab vocab.txt`.

Deploy without network access:

```text
/path/to/models/BASE2M/model.safetensors
/path/to/models/BASE2M/tokenizer.json
```

Then set:

```bash
export GO_POTION_HOME=/path/to/models
```

`go-potion` checks those local files first, so no download is performed when both files are already present. Keep the model at **64 dimensions**; the v1 SQLite vector schema is intentionally fixed to `int8[64]`.
