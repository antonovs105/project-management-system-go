import { useMemo, useState } from "react";
import type { ID, ProjectMember } from "../../types";
import { SelectField, TextField } from "../../components/ui";
import { memberLabel } from "./memberLabels";
import { useI18n } from "../../lib/i18n-context";

function matchesMember(member: ProjectMember, query: string): boolean {
  if (!query) {
    return true;
  }
  const haystack = [member.name, member.username, member.email, member.handle, member.role_name, member.user_id]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return haystack.includes(query.toLowerCase());
}

export function MemberAssigneeSelect({
  members,
  value,
  onChange,
  label,
}: {
  members: ProjectMember[];
  value: ID | "";
  onChange: (value: ID | "") => void;
  label?: string;
}) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const filteredMembers = useMemo(
    () => members.filter((member) => matchesMember(member, query.trim())),
    [members, query],
  );
  const selectedMember = members.find((member) => member.user_id === value);
  const options = selectedMember && !filteredMembers.some((member) => member.user_id === selectedMember.user_id)
    ? [selectedMember, ...filteredMembers]
    : filteredMembers;

  return (
    <div className="grid gap-2">
      <TextField
        label={t("ticket.findAssignee")}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder={t("ticket.findAssigneePlaceholder")}
      />
      <SelectField label={label || t("ticket.assignee")} value={value} onChange={(event) => onChange(event.target.value as ID | "")}>
        <option value="">{t("ticket.unassigned")}</option>
        {options.map((member) => (
          <option key={member.user_id} value={member.user_id}>
            {memberLabel(member)}
          </option>
        ))}
      </SelectField>
    </div>
  );
}
