{
  description = "http2 proxy using netfilter redir";
  nixConfig.bash-prompt = "[nix-develop:\\w]$ ";

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-22.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem ( system:
      let
          pkgs = nixpkgs.legacyPackages.${system};
      in rec {
        packages.mipsle = (pkgs.buildGoModule {
          pname = "http2-proxy-redir";
          version = "dev";
          src = ./.;
          vendorSha256 = "sha256-Qdyz6YdAfCwOHy2g/EzjsJGg2M41fbE6M8ydaLcgc58";
        }).overrideAttrs ( old: old // { GOOS="linux"; GOARCH = "mipsle"; CGO_ENABLED = 0; doCheck = false; } );

        packages.arm64 = (pkgs.buildGoModule {
          pname = "http2-proxy-redir";
          version = "dev";
          src = ./.;
          vendorSha256 = "sha256-Qdyz6YdAfCwOHy2g/EzjsJGg2M41fbE6M8ydaLcgc58";
        }).overrideAttrs ( old: old // { GOOS="linux"; GOARCH = "arm64"; CGO_ENABLED = 0; doCheck = false; } );

        packages.amd64 = (pkgs.buildGoModule {
          pname = "http2-proxy-redir";
          version = "dev";
          src = ./.;
          vendorSha256 = "sha256-Qdyz6YdAfCwOHy2g/EzjsJGg2M41fbE6M8ydaLcgc58";
        }).overrideAttrs ( old: old // { GOOS="linux"; GOARCH = "amd64"; CGO_ENABLED = 0; doCheck = false; } );

        defaultPackage = packages.mipsle;

        devShells.default = pkgs.mkShell {
          inputsFrom = [ packages.mipsle ];
          shellHook = ''
          '';
        };
      }
    );
}
