package sincro

import "testing"

func TestDebeBajar(t *testing.T) {
	for _, sinCopia := range []bool{false, true} {
		for _, local := range []bool{false, true} {
			for _, conocido := range []bool{false, true} {
				esperado := true
				if sinCopia && !local && !conocido {
					esperado = false
				}
				if got := debeBajar(sinCopia, local, conocido); got != esperado {
					t.Errorf("sinCopia=%v local=%v conocido=%v: %v", sinCopia, local, conocido, got)
				}
			}
		}
	}
}
