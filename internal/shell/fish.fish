function fish_command_not_found
    if not type -q adpm
        printf 'fish: Unknown command: %s\n' (string escape -- $argv[1]) >&2
        return 127
    end
    set -lx ADPM_SHELL fish
    command adpm --typo $argv
end

alias pacman 'adpm pacman'
alias yay 'adpm yay'
alias paru 'adpm paru'
alias apt 'adpm apt'
alias dnf 'adpm dnf'
alias brew 'adpm brew'
alias flatpak 'adpm flatpak'

function sudo
    if test (count $argv) -gt 0; and type -q adpm
        command adpm --ad-only $argv
        set result $status
        if test $result -eq 130
            return $result
        end
    end
    command sudo $argv
end
