import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "Centri",
  version: packageJson.version,
  copyright: `© ${currentYear} Barrier Networks.`,
  meta: {
    title: "Centri — Autonomous Penetration Testing Console",
    description: "Centri by Barrier Networks — console for the LLM-driven autonomous penetration testing platform",
  },
};
