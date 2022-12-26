{
  description = "http2 proxy using netfilter redir";

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-22.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem ( system:
      let
          pkgs = nixpkgs.legacyPackages.${system};
      in rec {
        packages.http2-proxy-redir-mips = pkgs.buildGoModule {
          pname = "http2-proxy-redir";
          version = "dev";
          src = ./.;
          vendorSha256 = "sha256-Qdyz6YdAfCwOHy2g/EzjsJGg2M41fbE6M8ydaLcgc58";
          overrideModAttrs = ( _: { GOOS="linux"; GOARCH = "arm64"; CGO_ENABLED = 0; doCheck = false; } );
        };

        defaultPackage = packages.http2-proxy-redir-mips;
      }
    );
}
