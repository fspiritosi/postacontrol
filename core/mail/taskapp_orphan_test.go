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
}
