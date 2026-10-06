# probe-convert

Converts tests from other frameworks into ProbeScript. Separate Go module (`github.com/alphawavesystems/probe-convert`).

```bash
cd tools/probe-convert && go build -o ../../bin/probe-convert .
```

`catalog/` and `convert/` hold the per-framework converters, `ui/` the interactive front end, `examples/` and
`testdata/` sample inputs. For Maestro flows prefer the built-in `probe migrate maestro`. Usage and supported
frameworks: [flutterprobe.dev/tools/probe-convert](https://flutterprobe.dev/tools/probe-convert/).
