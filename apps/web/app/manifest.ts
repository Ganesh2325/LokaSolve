import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "LokaSolve",
    short_name: "LokaSolve",
    description: "Report a local problem and follow it until the outcome is verified.",
    start_url: "/",
    display: "standalone",
    background_color: "#f3efe6",
    theme_color: "#0c6b56",
    icons: [{ src: "/icon.svg", sizes: "any", type: "image/svg+xml" }],
  };
}
