{inputs, ...}: {
  imports = [
    inputs.flake-root.flakeModule
    ./checks.nix
    ./devshell.nix
    ./dev
    ./formatter.nix
    ./packages.nix
  ];
}
