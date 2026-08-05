{ lib
, buildGoModule
, fetchFromGitHub
, makeWrapper
, python3
, cage
, umu-launcher
, bash
, coreutils
}:

let
  # Собираем Python-окружение прямо внутри пакета
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
in
buildGoModule rec {
  pname = "rbxdserver";
  version = "1.0.0";

  src = fetchFromGitHub {
    owner = "flaemer-idk";
    repo = "rbxdserver";
    rev = "main"; 
    hash = "sha256-5xMv72kzv7RDFQ8qUNZ856Tq5WBFP0p724fHb2HAjYg="; # Update this hash
  };

  vendorHash = "sha256-0Qxw+MUYVgzgWB8vi3HBYtVXSq/btfh4ZfV/m1chNrA="; # Update this hash

  subPackages = [ "cmd/rbxdserver" ];

  # Добавляем makeWrapper для создания обертки
  nativeBuildInputs = [ makeWrapper ];

  # Оборачиваем бинарник, жестко прописывая ему PATH со всеми зависимостями.
  # Xwayland удален.
  postInstall = ''
    wrapProgram $out/bin/rbxdserver \
      --prefix PATH : ${lib.makeBinPath [ pythonEnv cage umu-launcher bash coreutils ]}
  '';

  meta = with lib; {
    description = "Headless Boblox server daemon";
    homepage = "https://github.com/flaemer-idk/rbxdserver";
    license = licenses.mit;
    platforms = platforms.linux;
  };
}