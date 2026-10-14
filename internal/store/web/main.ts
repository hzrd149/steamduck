import "./crypto";
import "./store.css";
import "./account.css";
import "./settings.css";
import { mount } from "svelte";
import App from "./App.svelte";
import { installLegacyAdapter } from "./legacy";

if (import.meta.env.MODE === "legacy") installLegacyAdapter();

mount(App, { target: document.getElementById("app")! });
