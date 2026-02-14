"""Generate expanded PDF documents for Phase 3b.5 corpus (docs 12, 13).

These are table-heavy PDFs to test PDF table extraction.
Requires fpdf2: pip install fpdf2
"""
from __future__ import annotations

import sys
from pathlib import Path

try:
    from fpdf import FPDF
except ImportError:
    FPDF = None
    print("WARNING: fpdf2 not installed. PDF generation skipped.", file=sys.stderr)

OUTPUT_DIR = Path(__file__).parent.parent / "data" / "test-corpus"


def _pdf_table(pdf: "FPDF", headers: list[str], rows: list[list[str]], col_widths: list[float] | None = None) -> None:
    """Add a simple table to a PDF."""
    if col_widths is None:
        page_w = pdf.w - pdf.l_margin - pdf.r_margin
        col_widths = [page_w / len(headers)] * len(headers)

    # Header row
    pdf.set_font("Helvetica", "B", 7)
    for i, h in enumerate(headers):
        pdf.cell(col_widths[i], 6, h, border=1)
    pdf.ln()

    # Data rows
    pdf.set_font("Helvetica", "", 7)
    for row in rows:
        for i, cell in enumerate(row):
            pdf.cell(col_widths[i], 5, cell, border=1)
        pdf.ln()


def generate_compensation_pdf() -> None:
    """Doc 12: Employee Compensation Bands (PDF, table-heavy)."""
    if FPDF is None:
        print("  SKIPPED: employee_compensation_bands.pdf (fpdf2 not installed)")
        return

    pdf = FPDF()
    pdf.set_auto_page_break(auto=True, margin=15)
    pdf.add_page()

    pdf.set_font("Helvetica", "B", 14)
    pdf.cell(0, 10, "Acme Corp Employee Compensation Bands", ln=True, align="C")
    pdf.set_font("Helvetica", "", 8)
    pdf.cell(0, 5, "Document ID: HR-COMP-001 | Version: 2026.1 | Effective: January 1, 2026 | Confidential", ln=True, align="C")
    pdf.ln(4)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "1. Overview", ln=True)
    pdf.set_font("Helvetica", "", 9)
    pdf.multi_cell(0, 4,
        "Compensation bands are reviewed annually based on market data, cost of living, "
        "and company performance. Individual placement depends on experience, performance, "
        "and internal equity. Total compensation includes base salary, equity (RSUs), "
        "and performance bonus."
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "2. Engineering Bands", ln=True)
    _pdf_table(pdf,
        ["Level", "Title", "Base Salary", "Equity/yr", "Bonus"],
        [
            ["IC1", "Software Engineer I", "$95K-$120K", "$10K-$20K", "5%"],
            ["IC2", "Software Engineer II", "$120K-$155K", "$20K-$40K", "8%"],
            ["IC3", "Senior Software Eng", "$155K-$195K", "$40K-$70K", "10%"],
            ["IC4", "Staff Software Eng", "$195K-$240K", "$70K-$120K", "12%"],
            ["IC5", "Principal Engineer", "$240K-$300K", "$120K-$200K", "15%"],
            ["IC6", "Distinguished Eng", "$300K-$380K", "$200K-$350K", "20%"],
            ["M1", "Engineering Manager", "$170K-$210K", "$50K-$90K", "12%"],
            ["M2", "Sr Eng Manager", "$210K-$260K", "$90K-$150K", "15%"],
            ["D1", "Director of Eng", "$260K-$320K", "$150K-$250K", "18%"],
            ["VP", "VP of Engineering", "$320K-$400K", "$250K-$500K", "25%"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "3. Product & Design Bands", ln=True)
    _pdf_table(pdf,
        ["Level", "Title", "Base Salary", "Equity/yr", "Bonus"],
        [
            ["IC1", "Associate PM", "$85K-$110K", "$8K-$15K", "5%"],
            ["IC2", "Product Manager", "$110K-$145K", "$15K-$35K", "8%"],
            ["IC3", "Senior PM", "$145K-$185K", "$35K-$65K", "10%"],
            ["IC4", "Principal/Group PM", "$185K-$230K", "$65K-$110K", "12%"],
            ["IC2", "Product Designer", "$100K-$135K", "$12K-$30K", "8%"],
            ["IC3", "Sr Product Designer", "$135K-$175K", "$30K-$55K", "10%"],
            ["D1", "Director of Product", "$230K-$290K", "$110K-$200K", "18%"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "4. Sales & Marketing Bands", ln=True)
    _pdf_table(pdf,
        ["Level", "Title", "Base Salary", "OTE/Equity", "Bonus/Commission"],
        [
            ["IC1", "Sales Dev Rep (SDR)", "$55K-$70K", "OTE $85K-$110K", "Variable"],
            ["IC2", "Account Executive", "$80K-$110K", "OTE $160K-$220K", "Variable"],
            ["IC3", "Sr Account Executive", "$110K-$140K", "OTE $220K-$300K", "Variable"],
            ["IC2", "Marketing Manager", "$90K-$120K", "$12K-$25K RSU", "8%"],
            ["IC3", "Sr Marketing Manager", "$120K-$155K", "$25K-$50K RSU", "10%"],
            ["D1", "Director of Sales", "$160K-$200K", "OTE $320K-$450K", "Variable"],
            ["D1", "Director of Marketing", "$170K-$220K", "$80K-$150K RSU", "15%"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "5. Operations & Support Bands", ln=True)
    _pdf_table(pdf,
        ["Level", "Title", "Base Salary", "Equity/yr", "Bonus"],
        [
            ["IC1", "Support Specialist", "$50K-$65K", "$3K-$8K", "5%"],
            ["IC2", "Sr Support Specialist", "$65K-$85K", "$8K-$15K", "8%"],
            ["IC3", "Support Team Lead", "$85K-$105K", "$15K-$25K", "10%"],
            ["IC1", "Office Coordinator", "$45K-$60K", "$2K-$5K", "5%"],
            ["IC2", "Operations Analyst", "$70K-$95K", "$10K-$20K", "8%"],
            ["IC3", "Sr Ops Analyst", "$95K-$125K", "$20K-$40K", "10%"],
            ["M1", "Support Manager", "$105K-$135K", "$25K-$45K", "12%"],
            ["D1", "Director of Ops", "$150K-$200K", "$60K-$120K", "15%"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "6. Geographic Adjustments", ln=True)
    pdf.set_font("Helvetica", "", 9)
    pdf.multi_cell(0, 4,
        "Base salary bands are adjusted by geographic tier. The bands above reflect "
        "Tier 1 (San Francisco, New York, Seattle). Adjustments apply as follows:"
    )
    pdf.ln(1)
    _pdf_table(pdf,
        ["Geo Tier", "Locations", "Adjustment"],
        [
            ["Tier 1", "SF Bay Area, NYC, Seattle", "100% (reference)"],
            ["Tier 2", "LA, Boston, DC, Chicago, Austin", "90-95%"],
            ["Tier 3", "Denver, Portland, Atlanta, Miami", "85-90%"],
            ["Tier 4", "Other US metros", "80-85%"],
            ["Tier 5", "Rural US / low-cost areas", "75-80%"],
            ["International", "Varies by country", "Country-specific bands"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "7. Promotion & Adjustment Guidelines", ln=True)
    pdf.set_font("Helvetica", "", 9)
    pdf.multi_cell(0, 4,
        "Promotions typically include a 10-15% base salary increase. Annual merit increases "
        "range from 3-5% for meets expectations, 5-8% for exceeds, and 8-12% for exceptional. "
        "Market adjustments are made when an employee falls below the 25th percentile of their "
        "band. Equity refresh grants are awarded annually based on performance and retention risk."
    )

    pdf.output(str(OUTPUT_DIR / "employee_compensation_bands.pdf"))
    print("  Created: employee_compensation_bands.pdf")


def generate_asset_inventory_pdf() -> None:
    """Doc 13: IT Asset Inventory Standards (PDF, table-heavy)."""
    if FPDF is None:
        print("  SKIPPED: it_asset_inventory_standards.pdf (fpdf2 not installed)")
        return

    pdf = FPDF()
    pdf.set_auto_page_break(auto=True, margin=15)
    pdf.add_page()

    pdf.set_font("Helvetica", "B", 14)
    pdf.cell(0, 10, "Acme Corp IT Asset Inventory Standards", ln=True, align="C")
    pdf.set_font("Helvetica", "", 8)
    pdf.cell(0, 5, "Document ID: IT-AST-001 | Version: 3.0 | Effective: January 1, 2026 | Owner: IT Operations", ln=True, align="C")
    pdf.ln(4)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "1. Purpose", ln=True)
    pdf.set_font("Helvetica", "", 9)
    pdf.multi_cell(0, 4,
        "This document defines the standards for tracking, managing, and disposing of "
        "IT assets at Acme Corp. All hardware and software assets must be registered in "
        "the IT Asset Management System (ITAMS) within 24 hours of procurement."
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "2. Hardware Asset Categories", ln=True)
    _pdf_table(pdf,
        ["Category", "Standard Model", "Refresh Cycle", "Cost Range", "Depreciation"],
        [
            ["Laptop (Engineering)", "MacBook Pro 16\" M4 Pro", "3 years", "$2,800-$3,500", "Straight-line 3yr"],
            ["Laptop (Business)", "MacBook Air 15\" M4", "4 years", "$1,500-$2,000", "Straight-line 4yr"],
            ["Desktop (Specialized)", "Mac Studio M4 Ultra", "4 years", "$3,500-$5,000", "Straight-line 4yr"],
            ["Monitor", "Dell U2723QE 27\" 4K", "5 years", "$500-$700", "Straight-line 5yr"],
            ["Docking Station", "CalDigit TS4", "5 years", "$350-$400", "Straight-line 5yr"],
            ["Keyboard/Mouse", "Apple Magic Keyboard + Mouse", "3 years", "$200-$300", "Expensed"],
            ["Headset", "Jabra Evolve2 75", "3 years", "$250-$350", "Expensed"],
            ["Mobile Phone", "iPhone 16 Pro (company-issued)", "2 years", "$1,000-$1,200", "Straight-line 2yr"],
            ["Server (On-Prem)", "Dell PowerEdge R760", "5 years", "$8,000-$25,000", "Straight-line 5yr"],
            ["Network Switch", "Cisco Catalyst 9300", "7 years", "$5,000-$15,000", "Straight-line 7yr"],
            ["Access Point", "Cisco Meraki MR56", "5 years", "$1,000-$1,500", "Straight-line 5yr"],
            ["UPS", "APC Smart-UPS 3000VA", "5 years", "$1,500-$3,000", "Straight-line 5yr"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "3. Software Asset Categories", ln=True)
    _pdf_table(pdf,
        ["Category", "Examples", "License Type", "Annual Cost/User", "Tracking"],
        [
            ["Productivity", "Google Workspace", "SaaS subscription", "$144/user", "ITAMS + Google Admin"],
            ["Communication", "Slack Business+", "SaaS subscription", "$150/user", "ITAMS + Slack Admin"],
            ["Development", "GitHub Enterprise", "SaaS subscription", "$252/user", "ITAMS + GitHub Admin"],
            ["Project Mgmt", "Jira/Confluence", "SaaS subscription", "$168/user", "ITAMS + Atlassian Admin"],
            ["Design", "Figma Enterprise", "SaaS subscription", "$900/user", "ITAMS + Figma Admin"],
            ["Security", "CrowdStrike Falcon", "SaaS subscription", "$180/endpoint", "ITAMS + CS Console"],
            ["IDE", "JetBrains All Products", "Annual license", "$649/user", "ITAMS + JB Account"],
            ["Cloud Infra", "AWS", "Usage-based", "Varies", "AWS Cost Explorer + ITAMS"],
            ["Monitoring", "Datadog", "SaaS subscription", "$180/host", "ITAMS + DD Admin"],
            ["Secrets Mgmt", "HashiCorp Vault", "Enterprise license", "$1,200/user", "ITAMS"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "4. Asset Lifecycle", ln=True)
    _pdf_table(pdf,
        ["Phase", "Actions", "Responsible", "Timeline"],
        [
            ["Procurement", "Purchase order, vendor selection, budget approval", "IT Ops + Finance", "5-10 business days"],
            ["Receiving", "Inspect, register in ITAMS, apply asset tag", "IT Ops", "Within 24 hours"],
            ["Configuration", "Image, install software, security baseline", "IT Ops", "1-2 business days"],
            ["Deployment", "Assign to user, update ITAMS, user acknowledgment", "IT Ops", "Same day"],
            ["Maintenance", "Patches, repairs, warranty claims", "IT Ops", "Ongoing"],
            ["Reallocation", "Wipe, reconfigure, reassign, update ITAMS", "IT Ops", "1-2 business days"],
            ["Retirement", "Data wipe (DoD 5220.22-M), deregister, recycle/dispose", "IT Ops + Security", "Within 30 days"],
        ],
    )
    pdf.ln(3)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "5. Inventory Audits", ln=True)
    pdf.set_font("Helvetica", "", 9)
    pdf.multi_cell(0, 4,
        "Physical inventory audits are conducted quarterly for high-value assets (>$2,000) "
        "and annually for all assets. Software license audits are conducted semi-annually. "
        "Discrepancies must be investigated and resolved within 10 business days. The IT "
        "Asset Manager reports audit results to the CIO monthly."
    )
    pdf.ln(2)

    pdf.set_font("Helvetica", "B", 11)
    pdf.cell(0, 7, "6. Disposal Standards", ln=True)
    _pdf_table(pdf,
        ["Asset Type", "Data Sanitization", "Disposal Method", "Documentation"],
        [
            ["Laptops/Desktops", "DoD 5220.22-M 3-pass wipe", "Certified e-waste recycler", "Certificate of destruction"],
            ["Mobile Phones", "Factory reset + remote wipe", "Certified e-waste recycler", "Certificate of destruction"],
            ["Hard Drives/SSDs", "Physical destruction (shredding)", "Certified destruction vendor", "Certificate + serial numbers"],
            ["Servers", "DoD wipe + physical destruction of drives", "Certified e-waste recycler", "Certificate + asset deregistration"],
            ["Network Equipment", "Factory reset, remove configs", "Certified e-waste recycler", "Certificate of recycling"],
            ["Monitors/Peripherals", "N/A (no data)", "Certified e-waste recycler", "Recycling receipt"],
        ],
    )

    pdf.output(str(OUTPUT_DIR / "it_asset_inventory_standards.pdf"))
    print("  Created: it_asset_inventory_standards.pdf")


def generate_all() -> None:
    generate_compensation_pdf()
    generate_asset_inventory_pdf()
