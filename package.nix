{ lib
, buildGoModule
, fetchFromGitHub
, makeWrapper
, python3
, cage
, wineWow64Packages
, bash
, coreutils
}:

let
  pythonEnv = python3.withPackages (ps: with ps; [
    pygobject3
    websocket-client
    requests
    trustme
    urllib3
    pyzstd
    py7zr
    lz4
  ]);
  wine = wineWow64Packages.stable;
in
buildGoModule {
  pname = "rbxdserver";
  version = "1.0.0";

  src = fetchFromGitHub {
    owner = "flaemer-idk";
    repo = "rbxdserver";
    rev = "main"; 
    hash = "sha256-/yAOzYrBZPrlMsfx1eJM+NTFjP4fafjlXWZ7THtZXZk=";
  };

  vendorHash = "sha256-0Qxw+MUYVgzgWB8vi3HBYtVXSq/btfh4ZfV/m1chNrA=";

  subPackages = [ "cmd/rbxdserver" ];
  nativeBuildInputs = [ makeWrapper ];
  postInstall = ''
    wrapProgram $out/bin/rbxdserver \
      --prefix PATH : ${lib.makeBinPath [ pythonEnv cage wine bash coreutils ]}
  '';

  meta = with lib; {
    description = "Headless Boblox server daemon";
    homepage = "https://github.com/flaemer-idk/rbxdserver";
    license = licenses.mit;
    platforms = platforms.linux;
  };
}
