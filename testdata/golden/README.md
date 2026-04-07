# Golden Outputs

Use this directory for stable expected outputs:

- canonical bundle JSON
- doctor reports
- exporter manifests
- CLI snapshots

To refresh goldens locally:

```bash
SESSIONPORT_UPDATE_GOLDEN=1 go test ./...
```
