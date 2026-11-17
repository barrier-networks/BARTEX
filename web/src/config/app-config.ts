import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "Barrier",
  version: packageJson.version,
  copyright: `© ${currentYear}, Barrier.`,
  meta: {
    title: "Barrier — Autonomous Penetration Testing Console",
    description: "Console for the LLM-driven autonomous penetration testing system",
  },
};
