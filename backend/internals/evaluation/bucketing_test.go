package evaluation

import "testing"

func TestBucket_Deterministic(t *testing.T) {
	// Isti ulaz mora uvek dati isti izlaz.
	cases := []struct {
		userID  string
		flagKey string
	}{
		{"user-42", "checkout"},
		{"user-1", "new-ui"},
		{"", "flag-a"},
		{"user-42", ""},
	}
	for _, c := range cases {
		first := Bucket(c.userID, c.flagKey)
		for i := 0; i < 100; i++ {
			if got := Bucket(c.userID, c.flagKey); got != first {
				t.Fatalf("not deterministic: user=%q flag=%q first=%d got=%d (iteration %d)",
					c.userID, c.flagKey, first, got, i)
			}
		}
	}
}

func TestBucket_Range(t *testing.T) {
	// Bucket mora biti u [0, 99].
	for i := 0; i < 1000; i++ {
		user := "user-" + string(rune('a'+i%26))
		flag := "flag-" + string(rune('a'+i%17))
		b := Bucket(user, flag)
		if b < 0 || b > 99 {
			t.Fatalf("bucket out of range: %d", b)
		}
	}
}

func TestBucket_DifferentFlagsDifferentBuckets(t *testing.T) {
	// Isti korisnik, različiti flagovi — trebalo bi da daju različite
	// buckete bar u nekim slučajevima (ne očekujemo 100% različitost,
	// ali ne smemo dobiti sve iste).
	user := "user-42"
	buckets := map[int]int{}
	for _, flag := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		buckets[Bucket(user, flag)]++
	}
	if len(buckets) < 5 {
		t.Fatalf("expected more distinct buckets, got %d: %v", len(buckets), buckets)
	}
}

func TestBucket_DistributionRoughlyUniform(t *testing.T) {
	// Grubi test raspodele — 10.000 korisnika treba da bude relativno
	// ravnomerno raspoređeno u 10 grupa po 10%.
	const n = 10000
	groups := [10]int{}
	for i := 0; i < n; i++ {
		user := "user-" + itoa(i)
		b := Bucket(user, "flag-x")
		groups[b/10]++
	}
	// Očekujemo ~1000 po grupi. Dozvoljavamo ±30%.
	lo, hi := n/10*7/10, n/10*13/10
	for i, c := range groups {
		if c < lo || c > hi {
			t.Fatalf("group %d has %d, expected ~%d (allowed %d-%d)", i, c, n/10, lo, hi)
		}
	}
}

func TestShouldServe_ZeroPercent(t *testing.T) {
	for i := 0; i < 100; i++ {
		if ShouldServe("u-"+itoa(i), "f", 0) {
			t.Fatalf("0%% must never serve")
		}
	}
}

func TestShouldServe_HundredPercent(t *testing.T) {
	for i := 0; i < 100; i++ {
		if !ShouldServe("u-"+itoa(i), "f", 100) {
			t.Fatalf("100%% must always serve")
		}
	}
}

func TestShouldServe_MonotonicGrowth(t *testing.T) {
	// Ključna osobina: povećanje procenta ne sme da izbaci postojeće
	// korisnike iz grupe. Ako user ima bucket 7, u grupi je ako je
	// percent > 7. Kad percent raste, i dalje je unutra.
	users := make([]string, 500)
	for i := range users {
		users[i] = "user-" + itoa(i)
	}

	inGroup := map[string]bool{}
	for _, p := range []int{0, 5, 10, 20, 40, 60, 80, 100} {
		for _, u := range users {
			served := ShouldServe(u, "flag-x", p)
			if inGroup[u] && !served {
				t.Fatalf("user %q fell out of group at percent %d", u, p)
			}
			if served {
				inGroup[u] = true
			}
		}
	}
}

func TestShouldServe_NegativePercent(t *testing.T) {
	if ShouldServe("u", "f", -5) {
		t.Fatalf("negative percent must not serve")
	}
}

// itoa je mini helper da izbegnemo import "strconv".
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
