import { test, expect } from './fixtures/roles'; // fixtures login ต่อ role
import { actions } from './fixtures/actions';

// Full endpoint x role matrix from docs/spec/07-security-testing.md §2.1.
// qa-automation must keep every cell covered; a deny case is mandatory per role.
const matrix = [
  { role: 'viewer', action: 'publishFlow', allowed: false },
  { role: 'viewer', action: 'viewHistory', allowed: true },
  { role: 'operator', action: 'manualRun', allowed: true },
  { role: 'operator', action: 'editFlow', allowed: false },
  { role: 'designer', action: 'publishFlow', allowed: true },
  // ... generate ครบจาก 07-security-testing.md §2.1 (agent ต้อง cover ทุก cell)
] as const;

for (const { role, action, allowed } of matrix) {
  test(`RBAC: ${role} → ${action} = ${allowed ? 'allow' : 'deny'}`, async ({ pageAs }) => {
    const page = await pageAs(role);
    await expect(actions[action](page)).resolves.toBe(allowed);
  });
}
