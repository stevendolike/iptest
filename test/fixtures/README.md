# MaxMind regression fixtures

These are **test databases**, not production GeoLite2 data or live IP measurements.
All HTTP requests in the regression suite go to explicitly created loopback servers.
An IP string inside a trace fixture is only a lookup key in these local databases.

Vendored from `maxmind/MaxMind-DB` at commit
`0a9c1aa26cd7b91bee2efe27e7d174797d8211a6`, the test-data commit used by
`oschwald/geoip2-golang@v1.13.0`:

- `test-data/GeoLite2-ASN-Test.mmdb`
- `test-data/GeoLite2-Country-Test.mmdb`

`sources.json` records their original Git blob IDs, byte sizes and SHA256 hashes.
The source bytes were checked against both the original blob IDs and hashes when
vendored. `LICENSE-MaxMind` is the source repository's original MIT license;
MaxMind offers this repository under Apache-2.0 or MIT, and these fixtures use MIT.

The regression expectation for `89.160.20.112` comes from actually decoding these
pinned test databases: ASN `29518`, organization `Bredband2 AB`, country `瑞典`.
It is not a statement about that address's current real-world ownership or location.
