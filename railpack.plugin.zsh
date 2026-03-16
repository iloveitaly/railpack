# Railpack Zsh Plugin
# Loads completions dynamically from the railpack binary

if (( $+commands[railpack] )); then
    source <(railpack completion zsh)
fi
