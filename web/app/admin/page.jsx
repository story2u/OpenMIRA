import { AppShell } from "../../components/AppShell.jsx";
import { AdminDashboardClient } from "../../components/AdminDashboardClient.jsx";

export default function AdminPage() {
  return (
    <AppShell active="admin">
      <AdminDashboardClient />
    </AppShell>
  );
}
