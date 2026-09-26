package evaluation

import "hash/fnv"

// Bucket vraća deterministički broj 0-99 za par (userID, flagKey).
// Isti ulaz uvek daje isti izlaz, bez perzistencije.
func Bucket(userID, flagKey string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(userID))
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(flagKey))
	return int(h.Sum32() % 100)
}

// ShouldServe vraća true ako je korisnik unutar rollout procenta.
// Monoton rast: povećanje procenta ne izbacuje postojeće korisnike.
func ShouldServe(userID, flagKey string, rolloutPercent int) bool {
	if rolloutPercent <= 0 {
		return false
	}
	if rolloutPercent >= 100 {
		return true
	}
	return Bucket(userID, flagKey) < rolloutPercent
}
