{
  description = "http2 proxy using netfilter redir";
  nixConfig.bash-prompt = "[nix-develop:\\w]$ ";

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-24.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        rec {
          packages.mipsle = pkgs.buildGoModule {
            pname = "http2-proxy-redir";
            version = "dev";
            src = ./.;
            vendorHash = "sha256-0bBBhWtq0r2x5zmIAC6d9oK+G7FT2bC39bCUc2jxdtI=";
            doCheck = false;
            CGO_ENABLED = 0;
            preBuild = ''
              export GOOS=linux
              export GOARCH=mipsle
              export GOMIPS=softfloat
            '';
          };

          packages.arm64 = pkgs.buildGoModule {
            pname = "http2-proxy-redir";
            version = "dev";
            src = ./.;
            vendorHash = "sha256-0bBBhWtq0r2x5zmIAC6d9oK+G7FT2bC39bCUc2jxdtI=";
            doCheck = false;
            CGO_ENABLED = 0;
            preBuild = ''
              export GOOS=linux
              export GOARCH=arm
            '';
          };

          packages.amd64 = pkgs.buildGoModule {
            pname = "http2-proxy-redir";
            version = "dev";
            src = ./.;
            vendorHash = "sha256-0bBBhWtq0r2x5zmIAC6d9oK+G7FT2bC39bCUc2jxdtI=";
            doCheck = false;
            CGO_ENABLED = 0;
            preBuild = ''
              export GOOS=linux
              export GOARCH=amd64
            '';
          };

          defaultPackage = packages.mipsle;

          devShells.default = pkgs.mkShell {
            inputsFrom = [ packages.mipsle ];
            shellHook = ''
          '';
          };
        }
      ) // {
      formatter.x86_64-linux = nixpkgs.legacyPackages.x86_64-linux.nixpkgs-fmt;
      formatter.aarch64-darwin = nixpkgs.legacyPackages.aarch64-darwin.nixpkgs-fmt;
      formatter.x86_64-darwin = nixpkgs.legacyPackages.x86_64-darwin.nixpkgs-fmt;
    };
}
