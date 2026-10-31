command_not_found_handler() {
  if ! command -v adpm >/dev/null 2>&1; then
    printf 'zsh: command not found: %s\n' "$1" >&2
    return 127
  fi
  ADPM_SHELL=zsh command adpm --typo "$@"
}

alias pacman='adpm pacman'
alias yay='adpm yay'
alias paru='adpm paru'
alias apt='adpm apt'
alias dnf='adpm dnf'
alias brew='adpm brew'
alias flatpak='adpm flatpak'

sudo() {
  if (( $# > 0 )) && command -v adpm >/dev/null 2>&1; then
    command adpm --ad-only "$@"
    local result=$?
    if (( result == 130 )); then
      return "$result"
    fi
  fi
  command sudo "$@"
}
