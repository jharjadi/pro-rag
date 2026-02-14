"""Generate expanded HTML documents for Phase 3b.5 corpus (docs 9, 10, 11)."""
from __future__ import annotations
from pathlib import Path

OUTPUT_DIR = Path(__file__).parent.parent / "data" / "test-corpus"


def _html_table(headers: list[str], rows: list[list[str]]) -> str:
    lines = ['<table border="1">', "<tr>" + "".join(f"<th>{h}</th>" for h in headers) + "</tr>"]
    for row in rows:
        lines.append("<tr>" + "".join(f"<td>{c}</td>" for c in row) + "</tr>")
    lines.append("</table>")
    return "\n".join(lines)


def generate_vendor_management() -> None:
    """Doc 9: Vendor Management Policy (HTML)."""
    tiers = _html_table(
        ["Tier", "Criteria", "Due Diligence", "Review", "Examples"],
        [
            ["Tier 1 (Critical)", "Restricted/Confidential data OR essential ops",
             "Full security assessment, SOC 2, financial review", "Annual", "AWS, payroll, CRM"],
            ["Tier 2 (Important)", "Internal data OR key functions",
             "Security questionnaire, SOC 2/ISO 27001", "Biennial", "HR software, marketing"],
            ["Tier 3 (Standard)", "Public data, non-critical",
             "Registration, insurance verification", "At renewal", "Catering, cleaning"],
        ],
    )
    contracts = _html_table(
        ["Provision", "Tier 1", "Tier 2", "Tier 3"],
        [
            ["DPA", "Required", "If data access", "Not required"],
            ["SLA", "99.9%+ uptime", "99.5%+ uptime", "Optional"],
            ["Audit Rights", "Required", "Required", "Not required"],
            ["Breach Notice", "24 hours", "48 hours", "72 hours"],
            ["Insurance", "$5M cyber, $2M general", "$2M cyber, $1M general", "$1M general"],
            ["Data Return", "30 days", "60 days", "N/A"],
        ],
    )
    html = f"""<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Acme Corp Vendor Management Policy</title></head>
<body>
<h1>Acme Corp Vendor Management Policy</h1>
<p>Document ID: PROC-VM-001 | Version: 2.0 | Effective: January 15, 2026 | Owner: Procurement</p>

<h2>1. Purpose</h2>
<p>Requirements for selecting, onboarding, managing, and offboarding vendors. Critical for
service quality, cost control, data protection, and compliance.</p>

<h2>2. Vendor Classification</h2>
<p>Vendors classified by criticality and data sensitivity.</p>
{tiers}

<h2>3. Selection</h2>
<p>Contracts over $50K/year require formal RFP with 3+ vendors. Criteria: technical fit
(25-35%), cost (20-30%), security (15-25%), financial stability (10-15%), references
(5-10%), diversity (5-10%).</p>

<h2>4. Contract Requirements</h2>
{contracts}

<h2>5. Ongoing Management</h2>
<p>Quarterly reviews for Tier 1, semi-annual for Tier 2. Metrics: uptime, support response,
quality, security compliance, invoice accuracy. SLA failures trigger 90-day improvement plan.</p>

<h2>6. Risk Assessment</h2>
<p>Annual for Tier 1/2: security posture, financial health, BCP/DR, regulatory compliance,
concentration risk. Results reported to Risk Committee quarterly.</p>

<h2>7. Offboarding</h2>
<p>Within 30 days: revoke access, confirm data destruction, retrieve property, settle invoices,
update registry, lessons-learned for Tier 1.</p>
</body>
</html>"""
    (OUTPUT_DIR / "vendor_management_policy.html").write_text(html)
    print("  Created: vendor_management_policy.html")


def generate_acceptable_use() -> None:
    """Doc 10: Acceptable Use Policy (HTML)."""
    html = """<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Acme Corp Acceptable Use Policy</title></head>
<body>
<h1>Acme Corp Acceptable Use Policy</h1>
<p>Document ID: IT-AUP-001 | Version: 4.1 | Effective: January 1, 2026 | Owner: IT + CISO</p>

<h2>1. Purpose</h2>
<p>Defines acceptable use of Acme Corp IT resources: computers, networks, email, internet,
software, cloud services. Protects the company and employees from misuse risks.</p>

<h2>2. Acceptable Use</h2>
<p>IT resources are for business. Limited personal use OK if it does not interfere with work,
consume excessive resources, violate policy or law, or create security risks.</p>

<h3>2.1 Email</h3>
<p>Company email for all business communications. No personal email for business purposes.
No auto-forwarding to external accounts. Report suspicious emails to security immediately.</p>

<h3>2.2 Internet</h3>
<p>Filtered through corporate proxy. Blocked categories: adult content, gambling, malware,
anonymizing proxies, P2P file sharing. Social media OK for business and limited personal
use during breaks.</p>

<h3>2.3 Software</h3>
<p>Only approved software from IT catalog in Service Portal. Open-source needs Legal license
review. No pirated or unlicensed software under any circumstances.</p>

<h2>3. Prohibited Activities</h2>
<p>Unauthorized access (hacking, password cracking, port scanning); installing unauthorized
software or malware; sharing login credentials; circumventing security controls (disabling
antivirus, unauthorized VPNs); spam or phishing; downloading copyrighted material; personal
commercial use; cryptocurrency mining; accessing illegal content.</p>

<h2>4. Data Handling</h2>
<p>Follow classification levels per IT Security Policy (POL-SEC-001). Confidential and
Restricted data must not be stored on personal devices, shared via unapproved cloud services,
or transmitted without encryption. USB transfers of sensitive data require CISO approval.</p>

<h2>5. Monitoring</h2>
<p>All activity on company resources is monitored: email, internet, file access, application
usage. No expectation of privacy on company resources. Monitoring data retained 1 year,
accessible only to IT Security and HR with justification.</p>

<h2>6. Enforcement</h2>
<p>Violations: disciplinary action up to termination. Serious violations (data theft,
unauthorized access) may be referred to law enforcement. Report suspected violations
through the ethics hotline.</p>
</body>
</html>"""
    (OUTPUT_DIR / "acceptable_use_policy.html").write_text(html)
    print("  Created: acceptable_use_policy.html")


def generate_bcp() -> None:
    """Doc 11: Business Continuity Plan (HTML, table-heavy)."""
    funcs = _html_table(
        ["Function", "Dept", "MTD", "RTO", "RPO", "Priority"],
        [
            ["Customer SaaS platform", "Engineering", "4hr", "2hr", "15min", "P1"],
            ["Payment processing", "Finance", "4hr", "2hr", "Zero loss", "P1"],
            ["Customer support", "Support", "8hr", "4hr", "1hr", "P1"],
            ["Email/communication", "IT", "8hr", "4hr", "1hr", "P2"],
            ["Sales/CRM", "Sales", "24hr", "12hr", "4hr", "P2"],
            ["HR/payroll", "HR", "48hr", "24hr", "24hr", "P3"],
            ["Marketing website", "Marketing", "48hr", "24hr", "4hr", "P3"],
            ["Internal tools", "IT", "72hr", "48hr", "24hr", "P4"],
        ],
    )
    scenarios = _html_table(
        ["Scenario", "Probability", "Impact", "Risk", "Response"],
        [
            ["Data center outage", "Medium", "Critical", "High", "Failover to DR region"],
            ["Ransomware", "High", "Critical", "Critical", "Isolate + restore from backup"],
            ["Cloud provider outage", "Low", "Critical", "Medium", "Multi-region failover"],
            ["Pandemic", "Medium", "High", "High", "Full remote operations"],
            ["Natural disaster (HQ)", "Low", "High", "Medium", "Relocate to backup site"],
            ["Key vendor failure", "Medium", "Medium", "Medium", "Activate backup vendor"],
        ],
    )
    contacts = _html_table(
        ["Role", "Primary", "Backup", "Contact"],
        [
            ["Crisis Commander", "CEO Pat Morgan", "COO Sam Torres", "Phone + Slack #crisis"],
            ["IT Recovery", "CTO Alex Kim", "VP Infrastructure", "Phone + PagerDuty"],
            ["Communications", "VP Comms Jordan Lee", "PR Manager", "Phone + Slack"],
            ["Legal", "General Counsel", "Outside counsel", "Phone"],
            ["HR", "CHRO Maria Santos", "HR Director", "Phone + Slack"],
        ],
    )
    html = f"""<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Acme Corp Business Continuity Plan</title></head>
<body>
<h1>Acme Corp Business Continuity Plan</h1>
<p>Document ID: OPS-BCP-001 | Version: 3.0 | Effective: January 1, 2026 |
Owner: COO | Classification: Confidential</p>

<h2>1. Purpose</h2>
<p>Framework for maintaining critical operations during disruptive events: natural disasters,
technology failures, cyber attacks, pandemics.</p>

<h2>2. Critical Business Functions</h2>
{funcs}

<h2>3. Risk Scenarios</h2>
{scenarios}

<h2>4. Crisis Management Team</h2>
{contacts}

<h2>5. Recovery Strategies</h2>
<h3>5.1 Technology</h3>
<p>P1 systems: active-active across AWS us-east-1 and us-west-2. Synchronous replication for
payment systems, async (15-min lag) for others. Daily full backups, 30-day retention.
Quarterly recovery testing.</p>

<h3>5.2 Workforce</h3>
<p>All employees can work remotely within 4 hours. VPN supports 100% remote. Critical roles
have 2+ designated backups. Cross-training eliminates single points of failure.</p>

<h3>5.3 Communication</h3>
<p>Primary: Slack + email. Backup: SMS broadcast + phone tree. External: status page
(status.acmecorp.com), customer email, social media. Updates every 2hr for P1, 4hr for P2.</p>

<h2>6. Testing</h2>
<p>Quarterly tabletop exercises, annual full simulation. DR failover tested quarterly for P1.
Plan reviewed semi-annually. Annual BCP awareness training for all employees.</p>
</body>
</html>"""
    (OUTPUT_DIR / "business_continuity_plan.html").write_text(html)
    print("  Created: business_continuity_plan.html")


def generate_all() -> None:
    generate_vendor_management()
    generate_acceptable_use()
    generate_bcp()
