#!/usr/bin/env bash
#
# term-size.sh — mostra o tamanho do terminal em colunas x linhas.
#
# Rode ESTE script dentro do cool-retro-terminal, no monitor Hagibis,
# na resolução que você usa. Ele imprime quantas colunas e linhas cabem.
#
# Uso:
#   ./term-size.sh          # mostra o tamanho e desenha uma régua
#   ./term-size.sh -w       # fica observando: reimprime quando você
#                           # redimensiona a janela (Ctrl+C pra sair)

set -euo pipefail

# --- coleta as dimensões -----------------------------------------------------
# tput lê o terminal de verdade (respeita a janela atual do cool-retro-terminal).
get_dims() {
    COLS=$(tput cols)
    LINES=$(tput lines)
}

# --- desenha uma régua da largura exata --------------------------------------
draw_ruler() {
    local cols=$1
    # linha de números a cada 10 colunas: 1234567890123...
    local ruler=""
    local i=1
    while [ "$i" -le "$cols" ]; do
        ruler+=$(( i % 10 ))
        i=$(( i + 1 ))
    done
    printf '%s\n' "$ruler"
}

show() {
    get_dims
    clear
    printf '┌─ Terminal ────────────────────────────────┐\n'
    printf '│  Colunas (largura) : %-5s                 │\n' "$COLS"
    printf '│  Linhas  (altura)  : %-5s                 │\n' "$LINES"
    printf '│  Total de células  : %-8s              │\n' "$(( COLS * LINES ))"
    printf '└────────────────────────────────────────────┘\n'
    printf '\nRégua (cada dígito = 1 coluna, marcas a cada 10):\n'
    draw_ruler "$COLS"
    printf '\nTERM=%s\n' "${TERM:-desconhecido}"
}

# --- modo watch --------------------------------------------------------------
if [ "${1:-}" = "-w" ] || [ "${1:-}" = "--watch" ]; then
    trap 'printf "\nSaindo.\n"; exit 0' INT
    last=""
    while true; do
        cur="$(tput cols)x$(tput lines)"
        if [ "$cur" != "$last" ]; then
            show
            last="$cur"
        fi
        sleep 0.3
    done
else
    show
fi
