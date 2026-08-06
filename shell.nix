# shell.nix (для сервера rbxdserver)
{ pkgs ? import <nixpkgs> {} }:

let
  my-python = pkgs.python3.withPackages (ps: with ps; [
    pygobject3
    websocket-client
    requests
    trustme
    urllib3
    pyzstd
    py7zr
    lz4
  ]);
in
pkgs.mkShell {
  name = "rbxdserver-dev-shell";

  buildInputs = with pkgs; [
    # Инструменты сборки Go сервера
    go
    
    # Python-окружение для RFD
    my-python
    
    # Зависимости времени выполнения (без umu-launcher)
    cage
    wineWow64Packages.stable
    winetricks
    cabextract
    unzip
    p7zip
  ];

  shellHook = ''
    echo "===================================================="
    echo "  Welcome to the rbxdserver development environment! "
    echo "===================================================="
    echo " • Go version:     $(go version)"
    echo " • Python version: $(python3 --version)"
    echo ""
    echo " To run the server locally:"
    echo "   go run ./cmd/rbxdserver --rfd <path-to-rfd> --places <path-to-places>"
    echo "===================================================="
  '';
}
