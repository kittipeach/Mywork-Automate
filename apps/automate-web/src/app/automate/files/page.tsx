import { TopBar } from '@/components/shell/TopBar';
import { Card, CardBody } from '@/components/ui/Card';

export default function FilesPage() {
  return (
    <>
      <TopBar title="My Files" />
      <main className="flex-1 overflow-auto p-6">
        <Card>
          <CardBody className="text-sm text-ink-muted">
            Generated files with signed download links appear here (E8-S3).
          </CardBody>
        </Card>
      </main>
    </>
  );
}
