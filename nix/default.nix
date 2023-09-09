{inputs, ...}: {
  imports = [
    inputs.flake-root.flakeModule
    ./checks.nix
    ./formatter.nix
    ./devshell.nix
    ./dev
  ];
}
