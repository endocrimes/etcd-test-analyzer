{ pkgs ? import <nixpkgs> {} }:
  pkgs.mkShell {
    nativeBuildInputs = with pkgs; [
      go_1_17
      gotestsum
      golangci-lint
    ];
}

