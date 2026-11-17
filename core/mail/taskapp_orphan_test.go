package mail

import (
	"fmt"
	"strings"
	"testing"
)

// La UI reconoce este mensaje por la frase "no la vuelvas a crear".
func TestOrphanMessageContract(t *testing.T) {
	if !strings.Contains(fmt.Sprintf(msgTaskappOrphan, 7), "no la vuelvas a crear") {
		t.Fatal("el mensaje de huérfana perdió la frase que reconoce la UI")
	}
	// Si no se sabe si la tarea se creó, la UI la trata igual que la huérfana.
	if !strings.Contains(msgTaskappUncertain, "no la vuelvas a crear") {
		t.Fatal("el mensaje de creación incierta perdió la frase que reconoce la UI")
	}
}
