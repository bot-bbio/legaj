import json
import sys
import os
from reportlab.lib.pagesizes import letter
from reportlab.lib import colors
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle

def generate_elements(profile, base_font_size, leading, spacer_height, printable_width):
    styles = getSampleStyleSheet()
    
    # Proportional font sizes based on base_font_size
    name_size = base_font_size + 8.5
    contact_size = base_font_size
    section_size = base_font_size + 1.5
    item_header_size = base_font_size + 0.5
    body_size = base_font_size
    
    # Custom styles
    name_style = ParagraphStyle(
        'ResumeName',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=name_size,
        leading=name_size * 1.25,
        alignment=1, # Center
        spaceAfter=3
    )
    
    contact_style = ParagraphStyle(
        'ResumeContact',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=contact_size,
        leading=contact_size * 1.2,
        alignment=1, # Center
        spaceAfter=8
    )
    
    section_title_style = ParagraphStyle(
        'ResumeSectionTitle',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=section_size,
        leading=section_size * 1.2,
        spaceBefore=6,
        spaceAfter=2,
        alignment=1, # Center
        textColor=colors.HexColor("#000000") # Black
    )
    
    item_header_style = ParagraphStyle(
        'ResumeItemHeader',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=item_header_size,
        leading=item_header_size * 1.2,
        spaceAfter=1
    )
    
    item_subheader_style = ParagraphStyle(
        'ResumeItemSubheader',
        parent=styles['Normal'],
        fontName='Times-Italic',
        fontSize=body_size,
        leading=leading,
        spaceAfter=1
    )
    
    body_style = ParagraphStyle(
        'ResumeBody',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=body_size,
        leading=leading,
        spaceAfter=1
    )
    
    bullet_style = ParagraphStyle(
        'ResumeBullet',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=body_size,
        leading=leading,
        leftIndent=12,
        firstLineIndent=-8,
        spaceAfter=1.5
    )

    elements = []
    
    # 1. Header (Centered, bold name, followed by phone | email | socials - no location)
    info = profile.get("personal_info", {})
    elements.append(Paragraph(info.get("name", ""), name_style))
    
    contact_parts = []
    if info.get("phone"): contact_parts.append(info["phone"])
    if info.get("email"): contact_parts.append(info["email"])
    if info.get("linkedin"): contact_parts.append(info["linkedin"])
    if info.get("website"): contact_parts.append(info["website"])
    
    contact_str = " | ".join(contact_parts)
    elements.append(Paragraph(contact_str, contact_style))
    
    def draw_section_line(title):
        elements.append(Paragraph(title.upper(), section_title_style))
        # Use a table to draw a solid thin black border line matching printable width
        line_table = Table([[""]], colWidths=[printable_width], rowHeights=[1])
        line_table.setStyle(TableStyle([
            ('LINEBELOW', (0,0), (-1,-1), 1.0, colors.HexColor("#000000")),
            ('BOTTOMPADDING', (0,0), (-1,-1), 0),
            ('TOPPADDING', (0,0), (-1,-1), 0),
        ]))
        elements.append(line_table)
        elements.append(Spacer(1, spacer_height))
        
    # 2. Experience Section
    experience = profile.get("experience", [])
    if experience:
        draw_section_line("Professional Experience")
        for exp in experience:
            company = exp.get("company", "")
            dates = f"{exp.get('start_date', '')} - {exp.get('end_date', '')}"
            role = exp.get("role", "")
            loc = exp.get("location", "")

            # Line 1: Company (Left) and Location (Right) - Both Bold
            company_para = Paragraph(f"<b>{company}</b>", item_header_style)
            loc_para = Paragraph(f"<b>{loc}</b>", ParagraphStyle('RightLoc', parent=item_header_style, alignment=2))
            
            line1_table = Table([[company_para, loc_para]], colWidths=[printable_width * 0.72, printable_width * 0.28])
            line1_table.setStyle(TableStyle([
                ('VALIGN', (0,0), (-1,-1), 'BOTTOM'),
                ('LEFTPADDING', (0,0), (-1,-1), 0),
                ('RIGHTPADDING', (0,0), (-1,-1), 0),
                ('BOTTOMPADDING', (0,0), (-1,-1), 0),
                ('TOPPADDING', (0,0), (-1,-1), 1),
            ]))
            elements.append(line1_table)
            
            # Line 2: Role (Left) and Dates (Right)
            role_para = Paragraph(f"<i>{role}</i>", item_subheader_style)
            dates_para = Paragraph(dates, ParagraphStyle('RightDate', parent=item_subheader_style, alignment=2))
            
            line2_table = Table([[role_para, dates_para]], colWidths=[printable_width * 0.72, printable_width * 0.28])
            line2_table.setStyle(TableStyle([
                ('VALIGN', (0,0), (-1,-1), 'TOP'),
                ('LEFTPADDING', (0,0), (-1,-1), 0),
                ('RIGHTPADDING', (0,0), (-1,-1), 0),
                ('BOTTOMPADDING', (0,0), (-1,-1), 1),
                ('TOPPADDING', (0,0), (-1,-1), 0),
            ]))
            elements.append(line2_table)
                
            for bullet in exp.get("bullets", []):
                elements.append(Paragraph(f"&bull;&nbsp;&nbsp;{bullet}", bullet_style))
            elements.append(Spacer(1, spacer_height))
            
    # 3. Education and Research Section (Unified Section)
    education = profile.get("education", [])
    projects = profile.get("projects", [])
    
    # Filter projects to check if they look like research/publications
    research_projects = []
    other_projects = []
    for p in projects:
        p_name = p.get("name", "").lower()
        if "research" in p_name or "presentation" in p_name or "publication" in p_name or p.get("details"):
            research_projects.append(p)
        else:
            other_projects.append(p)

    if education or research_projects:
        if research_projects:
            draw_section_line("Education and Research")
        else:
            draw_section_line("Education")
            
        for edu in education:
            inst = edu.get("institution", "")
            deg = f"{edu.get('degree', '')} in {edu.get('major', '')}"
            dates = edu.get("graduation_date", "")
            loc = edu.get("location", "")
            gpa = f"GPA: {edu.get('gpa')}" if edu.get('gpa') else ""
            
            edu_header_text = f"<b>{inst}</b>"
            edu_loc_para = Paragraph(f"<b>{loc}</b>", ParagraphStyle('RightEduLoc', parent=item_header_style, alignment=2)) if loc else Paragraph("", item_header_style)
                
            edu_table = Table(
                [[Paragraph(edu_header_text, item_header_style), edu_loc_para]],
                colWidths=[printable_width * 0.72, printable_width * 0.28]
            )
            edu_table.setStyle(TableStyle([
                ('VALIGN', (0,0), (-1,-1), 'BOTTOM'),
                ('LEFTPADDING', (0,0), (-1,-1), 0),
                ('RIGHTPADDING', (0,0), (-1,-1), 0),
                ('BOTTOMPADDING', (0,0), (-1,-1), 0),
                ('TOPPADDING', (0,0), (-1,-1), 1),
            ]))
            elements.append(edu_table)
            
            deg_text = deg
            if gpa:
                deg_text += f" ({gpa})"
            if edu.get("details"):
                deg_text += f", {edu.get('details')}"
            
            line2_table = Table(
                [[Paragraph(f"<i>{deg_text}</i>", item_subheader_style), 
                  Paragraph(f"<i>{dates}</i>", ParagraphStyle('RightEduDate', parent=item_subheader_style, alignment=2))]],
                colWidths=[printable_width * 0.72, printable_width * 0.28]
            )
            line2_table.setStyle(TableStyle([
                ('VALIGN', (0,0), (-1,-1), 'TOP'),
                ('LEFTPADDING', (0,0), (-1,-1), 0),
                ('RIGHTPADDING', (0,0), (-1,-1), 0),
                ('BOTTOMPADDING', (0,0), (-1,-1), 1),
                ('TOPPADDING', (0,0), (-1,-1), 0),
            ]))
            elements.append(line2_table)
            elements.append(Spacer(1, spacer_height))
            
        # Draw research/publications under Education
        for proj in research_projects:
            name = proj.get("name", "")
            details = proj.get("details", "")
            
            elements.append(Paragraph(f"<b>{name}</b>", item_header_style))
            elements.append(Paragraph(details, body_style))
            elements.append(Spacer(1, spacer_height))

    # 4. Other Projects Section (if any non-academic projects exist)
    if other_projects:
        draw_section_line("Projects")
        for proj in other_projects:
            name = proj.get("name", "")
            tech = f"({', '.join(proj.get('technologies', []))})" if proj.get('technologies') else ""
            desc = proj.get("description", "")
            details = proj.get("details", "")
            
            proj_title = f"<b>{name}</b> {tech}" if tech else f"<b>{name}</b>"
            elements.append(Paragraph(proj_title, item_header_style))
            if desc:
                elements.append(Paragraph(desc, body_style))
            if details:
                elements.append(Paragraph(f"&bull;&nbsp;&nbsp;{details}", bullet_style))
            elements.append(Spacer(1, spacer_height))

    # 5. Additional Sections (intuited from résumé: Publications, Certifications,
    # Awards, etc.). Rendered verbatim, preserving the author's original titles.
    additional_sections = profile.get("additional_sections", []) or []
    for section in additional_sections:
        title = (section.get("title") or "").strip()
        items = section.get("items", []) or []
        if not title or not items:
            continue
        draw_section_line(title)
        for item in items:
            text = (item or "").strip()
            if not text:
                continue
            elements.append(Paragraph(f"&bull;&nbsp;&nbsp;{text}", bullet_style))
        elements.append(Spacer(1, spacer_height))

    # 6. Skills Section (Rendered beautifully at the bottom)
    skills = profile.get("skills", {})
    if skills:
        draw_section_line("Skills")
        skills_lines = []
        for cat, items in skills.items():
            cat_name = cat.replace("_", " ").title()
            skills_lines.append(f"<b>{cat_name}:</b> {', '.join(items)}")
            
        for line in skills_lines:
            elements.append(Paragraph(line, body_style))
            
    return elements

def build_resume_pdf(profile_path, output_path):
    try:
        with open(profile_path, 'r', encoding='utf-8') as f:
            profile = json.load(f)
    except Exception as e:
        print(f"Error loading profile: {str(e)}")
        sys.exit(1)
        
    # Visual locking presets to ensure single-page limit
    presets = [
        {"base_font_size": 9.5, "leading": 12.0, "margin": 36, "spacer": 2.0},  # Standard layout
        {"base_font_size": 9.0, "leading": 11.5, "margin": 32, "spacer": 1.5},  # Compact
        {"base_font_size": 8.5, "leading": 10.5, "margin": 28, "spacer": 1.0},  # Highly compressed
    ]
    
    success = False
    for idx, pr in enumerate(presets):
        doc = SimpleDocTemplate(
            output_path, 
            pagesize=letter,
            rightMargin=pr["margin"],
            leftMargin=pr["margin"],
            topMargin=pr["margin"],
            bottomMargin=pr["margin"]
        )
        
        printable_width = 612 - (2 * pr["margin"])
        elements = generate_elements(profile, pr["base_font_size"], pr["leading"], pr["spacer"], printable_width)
        doc.build(elements)
        
        if doc.page == 1:
            print(f"Successfully generated 1-page resume PDF at: {output_path} (preset {idx+1})")
            success = True
            break
            
    if not success:
        print(f"Warning: Resume exceeded 1 page ({doc.page} pages) even with maximum visual lock compression.")
        
if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python generate_resume_pdf.py <profile.json> <output.pdf>")
        sys.exit(1)
    build_resume_pdf(sys.argv[1], sys.argv[2])
