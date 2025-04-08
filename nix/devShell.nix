pkgs: with pkgs; {
  # TODO: most of these really should be a check somehow
  #
  # For now I'm just including them in the devshell so I can test local pacakge
  # derivations.
  packages = with pkgs; [
    (pkgs.wrapHelm pkgs.kubernetes-helm { plugins = [ pkgs.kubernetes-helmPlugins.helm-diff ]; })
    pulumi-bin
    go_1_23
    kubectl
    k8sgpt
    jq
    libvirt
    helmfile-wrapped
    k9s
    cdrkit # for libvirt mkisofs
    gptfdisk
    skopeo
    kubernetes-helm
  ];
}
