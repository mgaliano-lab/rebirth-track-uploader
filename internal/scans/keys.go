package scans

// TrustedKeys son las claves públicas Ed25519 (32 bytes, base64) con que el sitio puede firmar el
// manifiesto de relevamientos. La privada está en el secret del sitio (NUXT_SCANS_SIGNING_KEY,
// Vault de OCI); se generan con `node scripts/scans-keygen.mjs` en raidtrack.
//
// Para rotar sin cortar: sumar la nueva acá, publicar la app y, cuando las PCs ya la tengan,
// firmar con la nueva y sacar la vieja en el release siguiente.
var TrustedKeys = []string{
	"URnOaSCL3fBnL7OnsHtbh76lrpuNfT9fpAe+Azkey1c=", // keyId 8e146879fb9a01fe (2026-10-22)
}
