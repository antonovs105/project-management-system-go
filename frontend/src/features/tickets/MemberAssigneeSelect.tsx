import { useMemo, useState } from "react";
import type { ID, ProjectMember } from "../../types";
import { SelectField, TextField } from "../../components/ui";

function memberLabel(member: ProjectMember): string {
  const primary = member.name || member.username || member.email || member.handle || member.user_id;
  const secondary = member.email && member.email !== primary ? member.email : member.handle;
  return secondary ? `${primary} / ${secondary}` : primary;
}

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
  label = "Assignee",
}: {
  members: ProjectMember[];
  value: ID | "";
  onChange: (value: ID | "") => void;
  label?: string;
}) {
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
        label="Find assignee"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Name, email, username, or handle"
      />
      <SelectField label={label} value={value} onChange={(event) => onChange(event.target.value as ID | "")}>
        <option value="">Unassigned</option>
        {options.map((member) => (
          <option key={member.user_id} value={member.user_id}>
            {memberLabel(member)}
          </option>
        ))}
      </SelectField>
    </div>
  );
}

export function projectMemberLabel(members: ProjectMember[], userID: ID | null | undefined): string {
  if (!userID) {
    return "Unassigned";
  }
  const member = members.find((item) => item.user_id === userID);
  return member ? memberLabel(member) : userID;
}
