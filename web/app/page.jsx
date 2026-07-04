import { AppShell } from "../components/AppShell.jsx";
import { WorkbenchClient } from "../components/WorkbenchClient.jsx";

export default function HomePage() {
  return (
    <AppShell active="cs">
      <WorkbenchClient />
    </AppShell>
  );
}
