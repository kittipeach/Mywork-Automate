import { TopBar } from '@/components/shell/TopBar';
import { Card, CardBody } from '@/components/ui/Card';

export default function AdminPage() {
  return (
    <>
      <TopBar title="Admin & RBAC" />
      <main className="flex-1 overflow-auto p-6">
        <Card>
          <CardBody className="text-sm text-ink-muted">
            Users, roles, flow grants, and masking rules (E2-S3 / E2-S4).
          </CardBody>
        </Card>
      </main>
    </>
  );
}
