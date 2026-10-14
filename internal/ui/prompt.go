// Package ui contiene el prompt del AID y las barras de progreso.
package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var reAID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidarAID comprueba que el AID no esté vacío y solo use caracteres esperados.
func ValidarAID(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("el AID no puede estar vacío")
	}
	if !reAID.MatchString(s) {
		return errors.New("el AID solo admite letras, números, guion y guion bajo (máximo 64 caracteres)")
	}
	return nil
}

// PedirAID solicita el AID por pantalla (hasta 3 intentos).
func PedirAID(in io.Reader, out io.Writer) (string, error) {
	lector := bufio.NewReader(in)
	for intento := 0; intento < 3; intento++ {
		fmt.Fprint(out, "Ingrese el AID (eventPoId) de la alarma: ")
		linea, err := lector.ReadString('\n')
		linea = strings.TrimSpace(linea)
		if err != nil && linea == "" {
			return "", fmt.Errorf("no se pudo leer el AID: %w", err)
		}
		if e := ValidarAID(linea); e != nil {
			fmt.Fprintf(out, "  AID inválido: %v\n", e)
			continue
		}
		return linea, nil
	}
	return "", errors.New("AID inválido tras 3 intentos")
}
