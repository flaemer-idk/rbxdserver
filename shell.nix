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
    # Server Build Tools
    go
    
    # Python Environment for RFD
    my-python
    
    # Run-time dependencies
    cage
    umu-launcher
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
