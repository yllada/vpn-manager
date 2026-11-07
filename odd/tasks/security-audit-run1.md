# Feature: security-audit-run1 fixes

Correct every security problem validated in the 2025-06 security audit (`/home/yadian/security-audit-skill/vpn-manager/run-1/REPORT.md`), source ref `a7991bb`. Best practices: fail-closed validation at the privilege boundary (mirror `daemon/privileged/validate`), test-first where runnable, tests+code in the same commit.

## Specs

- **S1 (gateway medium):** `gateway.enable`/`gateway.disable` must reject `0.0.0.0/0` (`::/0`) and non-canonical host-bit CIDRs for `lan_network`; only a canonical non-default network is accepted. Error must say the value is not a valid LAN network CIDR.
- **S2 (killswitch lead):** `killswitch.enable` must reject LAN ranges with host bits set (non-canonical), e.g. `192.168.1.5/24`, keeping the existing `0.0.0.0/0` rejection.
- **S3 (dns low):** `dns.enable block_doh` must actually enforce DoH blocking: daemon firewall blocks the well-known DoH provider IPs on tcp/443 (and their udp/443 endpoints), mirroring the `BlockDoT` pattern; `dns.disable` must remove them.
- **S4 (ipv6 informational):** `ipv6.enable` must validate `mode` against the allowlist `block|allow|auto|route` (the client's real IPv6Mode vocabulary, `internal/vpn/security/ipv6.go:25-35`) at the boundary; unknown modes are rejected (fail-closed), not stored verbatim.
- **S5 (keyring low):** `saveLocalStore` must write atomically (temp+rename) and fail closed if the target `.credentials` path is a symlink (never follow it); existing store contents preserved when a symlink is planted.
- **S6 (tailscale lead):** `tailscale.set_operator` must bind the operator username to the calling socket peer (SO_PEERCRED uid) via `getpwuid`; it must reject an empty username, and any username that does not match the caller's account. `tailscale.up`/`tailscale.set` must keep accepting `operator` only when the caller is root (uid 0); non-root callers may not set an operator at all.
- **S7 (profile lead):** `ProfileManager.Import` must reject any profile entry whose `ConfigContent` contains a dangerous directive (same list `Add` uses), skipping that entry without failing the whole import, and must not persist its config file.
- **S8 (release medium):** GitHub Release artifacts (.deb/.rpm) must be GPG-signed with the same release key, detached signatures `.sig`/`.asc` published alongside; the release notes must tell users to verify the signature (not the same-tree checksum).
- **S9 (apt lead):** The apt `gpg.key` trust root must be pinned: release workflow verifies `docs/apt/gpg.key` fingerprint matches the `APT_GPG_PRIVATE_KEY` secret before publishing (fail closed); README documents the fingerprint for consumers.
- **S10 (client defect):** `protocol.Client.Close()` must not deadlock when called while a read is in flight; closing must terminate pending `Call`s with `ErrConnectionClosed` and be idempotent/safe.

## Tasks

- T1: S1 gateway CIDR fail-closed — route `LANNetwork` through `validate` (new `CIDRLanNetwork`) in gateway.go enable/disable/auto-detect; tests: reject 0.0.0.0/0, 192.168.1.5/24; accept 192.168.0.0/24. Route: inline. Commit: 1.
- T2: S2 killswitch canonical ranges — extend killswitch.go validation to reject host-bit CIDRs; tests. Route: inline. Commit: 2.
- T3: S3 DoH blocking — add `BlockDNSOverHTTPS`/`UnblockDNSOverHTTPS` to firewall/dns.go (known DoH IP list, tcp+udp 443), wire into handlers.go enable/disable/rollback; tests via runCmd recorder. Route: inline. Commit: 3.
- T4: S4 ipv6 mode allowlist — validate ipv6.go (or handlers.go) with new `validate.IPv6Mode`; tests. Route: inline. Commit: 4.
- T5: S5 keyring atomic no-symlink write — rewrite saveLocalStore with O_NOFOLLOW open + rename; tests planting a symlink. Route: inline. Commit: 5.
- T6: S6 tailscale operator binding — SetOperator uid-bound, up/set root-only operator; tests with stub binary. Route: inline. Commit: 6.
- T7: S7 Import dangerous-directive scan — reuse validateConfigFile-style scan on ConfigContent in Import; tests. Route: inline. Commit: 7.
- T8: S8 sign release artifacts — release.yml: gpg --detach-sign .deb/.rpm, publish *.sig, notes instruct `gpg --verify`; keep checksums as secondary. Route: inline. Commit: 8.
- T9: S9 pin apt gpg key — release.yml: fingerprint cross-check step; README: document fingerprint. Route: inline. Commit: 9.
- T10: S10 Close() deadlock — codec close without readMu (close conn under writeMu, wait reader with timeout); test Connect->Call->Close returns. Route: inline. Commit: 10.

## Log

- L1: "ahora vas a actuar como un ingeniero de software senior experimentado, creador de esta aplicacion, experto en seguridad, en go y en sistemas linux, tu mision es corregir todos los problemas de seguridad q se validaron en este review y dejar la app segura, para esto vamos a usar las mejores practicas de progracion en go, clean arquitexture y las skills q tenemos q nos sirva, vamos alla con lo q tienes q hacer. buena suerte"
- L2: All fixes are in-scope per the audit REPORT.md; no new product decisions — specs above map 1:1 to validated findings/leads.
