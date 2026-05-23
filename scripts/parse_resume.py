import os
import sys

def parse_pdf(file_path):
    try:
        from pypdf import PdfReader
    except ImportError:
        return "Error: pypdf library is not installed."
        
    try:
        reader = PdfReader(file_path)
        text = ""
        for page in reader.pages:
            page_text = page.extract_text()
            if page_text:
                text += page_text + "\n"
        return text.strip()
    except Exception as e:
        return f"Error reading PDF file: {str(e)}"

def parse_docx(file_path):
    try:
        import docx
    except ImportError:
        return "Error: python-docx library is not installed."
        
    try:
        doc = docx.Document(file_path)
        text = ""
        for para in doc.paragraphs:
            text += para.text + "\n"
        for table in doc.tables:
            for row in table.rows:
                row_text = [cell.text for cell in row.cells]
                text += " | ".join(row_text) + "\n"
        return text.strip()
    except Exception as e:
        return f"Error reading DOCX file: {str(e)}"

def parse_txt(file_path):
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            return f.read()
    except Exception as e:
        return f"Error reading text file: {str(e)}"

def main():
    if len(sys.argv) < 2:
        print("Usage: python parse_resume.py <resume_path>")
        sys.exit(1)
        
    file_path = sys.argv[1]
    if not os.path.exists(file_path):
        print(f"Error: File '{file_path}' does not exist.")
        sys.exit(1)
        
    ext = os.path.splitext(file_path)[1].lower()
    
    if ext == '.pdf':
        text = parse_pdf(file_path)
    elif ext in ['.docx', '.doc']:
        text = parse_docx(file_path)
    elif ext in ['.txt', '.md']:
        text = parse_txt(file_path)
    else:
        print(f"Error: Unsupported file format '{ext}'. Supported formats: PDF, DOCX, TXT, MD.")
        sys.exit(1)
        
    print(text)

if __name__ == "__main__":
    main()
