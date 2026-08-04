{ lib, buildGoModule, fetchFromGitHub }:

buildGoModule rec {
  pname = "rbxdserver";
  version = "1.0.0";

  # Option A: Build from GitHub (recommended for automated deployments)
  src = fetchFromGitHub {
    owner = "flaemer-idk";
    repo = "rbxdserver";
    rev = "main"; # Or specify a git tag/commit hash here
    hash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="; # Update this hash
  };

  vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="; # Update this hash

  subPackages = [ "cmd/rbxdserver" ];

  meta = with lib; {
    description = "Headless Roblox server supervisor daemon";
    homepage = "https://github.com/flaemer-idk/rbxdserver";
    license = licenses.mit;
    platforms = platforms.linux;
  };
}
