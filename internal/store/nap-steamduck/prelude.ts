import { installSteamduckBinding, type ExtensionTransport } from "./binding";

// Lexical helper supplied by Kwakore's trusted preamble, not a window global.
declare const napExtension: ExtensionTransport;

// Runtime entrypoint only. The store application must never import this file.
// The IIFE build introduces no installer or lifecycle globals in the frame.
installSteamduckBinding(window, napExtension);
